package deltachat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deltachat/deltachat-rpc-client-go/deltachat"
	"github.com/deltachat/deltaircd/bridge"
	"github.com/forPelevin/gomoji"
	prefixed "github.com/matterbridge/logrus-prefixed-formatter"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type DeltaChat struct {
	account     *deltachat.Account
	credentials bridge.Credentials
	eventChan   chan<- *bridge.Event
	cfg         *viper.Viper
	onConnect   func()
	connected   bool
}

var (
	logger *logrus.Entry
	rpc    *deltachat.RpcIO
)

func New(cfg *viper.Viper, cred bridge.Credentials, eventChan chan<- *bridge.Event, onConnect func()) (bridge.Bridger, error) {
	dc := &DeltaChat{
		credentials: cred,
		eventChan:   eventChan,
		cfg:         cfg,
		onConnect:   onConnect,
	}

	ourlog := logrus.New()
	ourlog.SetFormatter(&prefixed.TextFormatter{
		PrefixPadding: 17,
		FullTimestamp: true,
	})
	logger = ourlog.WithFields(logrus.Fields{"prefix": "bridge/deltachat"})
	if cfg.GetBool("debug") {
		ourlog.SetLevel(logrus.DebugLevel)
	}

	if cfg.GetBool("trace") {
		ourlog.SetLevel(logrus.TraceLevel)
	}

	if err := dc.loginToDeltaChat(); err != nil {
		return nil, err
	}
	return dc, nil
}

func (self *DeltaChat) loginToDeltaChat() error {
	if rpc == nil {
		rpc = deltachat.NewRpcIO()
		path, err := homedir.Expand(self.cfg.GetString(self.Protocol() + ".accounts"))
		if err != nil {
			return err
		}
		rpc.AccountsDir = path
		rpc.Start()
	}

	manager := deltachat.AccountManager{rpc}
	accounts, _ := manager.Accounts()

	isBackupLink := strings.HasPrefix(self.credentials.Login, "DCBACKUP:")
	if isBackupLink {
		self.account, _ = manager.AddAccount()
	} else if self.credentials.Login != "" {
		for _, acc := range accounts {
			addr, _ := acc.GetConfig("addr")
			if addr == self.credentials.Login {
				self.account = acc
				break
			}
		}
	}

	if self.account == nil {
		if self.credentials.Pass != "" {
			self.account, _ = manager.AddAccount()
		} else if self.credentials.Login == "" && len(accounts) != 0 {
			acc, _ := manager.SelectedAccount()
			if acc == nil {
				acc = accounts[0]
			}
			self.account = acc
		} else {
			return fmt.Errorf("need LOGIN <email> <pass>")
		}
	}

	if isBackupLink {
		logger.Debugf("Configuring account from another device...")
		if err := self.account.GetBackup(self.credentials.Login); err != nil {
			return err
		} else {
			self.account.StartIO()
		}
	} else if self.credentials.Pass != "" {
		logger.Debugf("Configuring account %v...", self.credentials.Login)
		self.account.SetConfig("addr", self.credentials.Login)
		self.account.SetConfig("mail_pw", self.credentials.Pass)
		if err := self.account.Configure(); err != nil {
			return err
		}
	} else if configured, _ := self.account.IsConfigured(); configured {
		self.account.StartIO()
	} else {
		return fmt.Errorf("need LOGIN <email> <pass>")
	}

	go self.onConnect()
	go func() {
		self.processMessages() // process old messages
		self.handleEvents()
	}()

	self.connected = true
	return nil
}

func (self *DeltaChat) handleEvents() {
	eventChan := self.account.GetEventChannel()
	for {
		event, ok := <-eventChan
		if !ok {
			break
		}
		self.handleEvent(event)
	}
}

func (self *DeltaChat) handleEvent(event deltachat.Event) {
	switch ev := event.(type) {
	case deltachat.EventInfo:
		logger.Debug("INFO:", ev.Msg)
	case deltachat.EventWarning:
		logger.Debug("WARNING:", ev.Msg)
	case deltachat.EventError:
		logger.Debug("ERROR:", ev.Msg)
	case deltachat.EventReactionsChanged:
		msg := &deltachat.Message{self.account, ev.MsgId}
		msgData, err := msg.Snapshot()
		if err != nil {
			break
		}
		reactions := ""
		var reactionsMap map[string]int
		if msgData.Reactions != nil {
			reactionsMap = msgData.Reactions.Reactions
		}
		for k, v := range reactionsMap {
			reactions += fmt.Sprintf("(%v %v) ", replaceEmojisWithSlug(k), v)
		}
		if reactions == "" {
			reactions = "(no reactions)"
		}

		channelType := ""
		var user *bridge.UserInfo
		chat := &deltachat.Chat{self.account, ev.ChatId}
		chatData, err := chat.BasicSnapshot()
		if err == nil && chatData.ChatType == deltachat.ChatSingle {
			channelType = "D"
			contacts, _ := chat.Contacts()
			contact, _ := contacts[0].Snapshot()
			user = self.getUserInfo(contact)
		} else {
			user = self.GetMe()
		}

		bridgeEvent := &bridge.Event{
			Type: "reaction_add",
			Data: &bridge.ReactionAddEvent{
				ChannelID:   strconv.FormatUint(uint64(msgData.ChatId), 10),
				MessageID:   strconv.FormatUint(uint64(msgData.Id), 10),
				Sender:      user,
				Reaction:    reactions,
				ChannelType: channelType,
			},
		}
		self.eventChan <- bridgeEvent
	case deltachat.EventIncomingMsg:
		chat := &deltachat.Chat{self.account, ev.ChatId}
		chat.SetMuteDuration(0) // TODO: remove this when there is a way to get fresh messages from muted chats
		self.processMessages()
	case deltachat.EventMsgsChanged:
		if ev.MsgId != 0 {
			msg := &deltachat.Message{self.account, ev.MsgId}
			msgData, err := msg.Snapshot()
			if err == nil && msgData.IsInfo {
				self.processInfoMsg(msgData)
			}

		}
	}
}

func (self *DeltaChat) processInfoMsg(msgData *deltachat.MsgSnapshot) bool {
	switch msgData.SystemMessageType {
	case deltachat.SysmsgMemberAddedToGroup:
		actor, target, err := msgData.ParseMemberAdded()
		if err != nil {
			return false
		}
		added, err := target.Snapshot()
		if err != nil {
			return false
		}
		adder, err := actor.Snapshot()
		if err != nil {
			return false
		}
		event := &bridge.Event{
			Type: "channel_add",
			Data: &bridge.ChannelAddEvent{
				Added: []*bridge.UserInfo{
					self.getUserInfo(added),
				},
				Adder:     self.getUserInfo(adder),
				ChannelID: strconv.FormatUint(uint64(msgData.ChatId), 10),
			},
		}
		self.eventChan <- event
		return true
	case deltachat.SysmsgMemberRemovedFromGroup:
		actor, target, err := msgData.ParseMemberRemoved()
		if *target == *msgData.Account.Me() && *actor == *target {
			return true
		}
		if err != nil {
			return false
		}
		removed, err := target.Snapshot()
		if err != nil {
			return false
		}
		var remover *bridge.UserInfo
		if *actor != *target {
			removerData, err := actor.Snapshot()
			if err != nil {
				return false
			}
			remover = self.getUserInfo(removerData)
		}
		event := &bridge.Event{
			Type: "channel_remove",
			Data: &bridge.ChannelRemoveEvent{
				Removed: []*bridge.UserInfo{
					self.getUserInfo(removed),
				},
				Remover:   remover,
				ChannelID: strconv.FormatUint(uint64(msgData.ChatId), 10),
			},
		}
		self.eventChan <- event
		return true
	case deltachat.SysmsgGroupNameChanged:
		if msgData.FromId == deltachat.ContactSelf {
			return false
		}
		chat := &deltachat.Chat{msgData.Account, msgData.ChatId}
		chatData, err := chat.BasicSnapshot()
		if err != nil {
			return false
		}
		event := &bridge.Event{
			Type: "channel_topic",
			Data: &bridge.ChannelTopicEvent{
				Text:      chatData.Name,
				ChannelID: strconv.FormatUint(uint64(msgData.ChatId), 10),
				UserID:    strconv.FormatUint(uint64(msgData.FromId), 10),
			},
		}
		self.eventChan <- event
		return true
	default:
		return false
	}
}

func (self *DeltaChat) processMessages() {
	msgs, _ := self.account.FreshMsgsInArrivalOrder()
	logger.Debugf("Processing %v messages", len(msgs))
	for _, msg := range msgs {
		msgData, _ := msg.Snapshot()
		if !msgData.IsInfo || !self.processInfoMsg(msgData) {
			self.processMsg(msgData)
		}
		msg.MarkSeen()
	}
}

func (self *DeltaChat) processMsg(msgData *deltachat.MsgSnapshot) {
	logger.Debugf("Processing message (id=%v)", msgData.Id)

	ghost := self.getUserInfo(msgData.Sender)

	chat := deltachat.Chat{self.account, msgData.ChatId}
	chatData, _ := chat.BasicSnapshot()
	channelID := strconv.FormatUint(uint64(chat.Id), 10)

	text := msgData.Text
	if msgData.File != "" {
		if text != "" {
			text = "file://" + msgData.File + "\n" + text
		} else {
			text = "file://" + msgData.File
		}
	}
	if msgData.OverrideSenderName != "" {
		text = fmt.Sprintf("<%s> %s", msgData.OverrideSenderName, text)
	}

	msgId := strconv.FormatUint(uint64(msgData.Id), 10)
	quotedId := ""
	if msgData.Quote != nil && msgData.Quote.MessageId != 0 {
		quotedId = strconv.FormatUint(uint64(msgData.Quote.MessageId), 10)
	}

	if chatData.ChatType == deltachat.ChatSingle {
		self.sendDirectMessage(ghost, ghost, channelID, msgId, quotedId, text)
	} else {
		self.sendPublicMessage(ghost, channelID, msgId, quotedId, text)
	}
}

func (self *DeltaChat) sendDirectMessage(sender, receiver *bridge.UserInfo, channelID, msgID, parentID, text string) {
	for _, line := range strings.Split(text, "\n") {
		event := &bridge.Event{
			Type: "direct_message",
			Data: &bridge.DirectMessageEvent{
				Text:      line,
				Sender:    sender,
				Receiver:  receiver,
				ChannelID: channelID,
				MessageID: msgID,
				ParentID:  parentID,
			},
		}
		self.eventChan <- event
	}
}

func (self *DeltaChat) sendPublicMessage(ghost *bridge.UserInfo, channelID, msgID, parentID, text string) {
	for _, line := range strings.Split(text, "\n") {
		event := &bridge.Event{
			Type: "channel_message",
			Data: &bridge.ChannelMessageEvent{
				Text:      line,
				ChannelID: channelID,
				Sender:    ghost,
				MessageID: msgID,
				ParentID:  parentID,
			},
		}
		self.eventChan <- event
	}
}

func (self *DeltaChat) getUserInfo(dcuser *deltachat.ContactSnapshot) *bridge.UserInfo {
	if dcuser == nil {
		return &bridge.UserInfo{}
	}

	nick := strings.ReplaceAll(dcuser.Address, "@", "|")

	return &bridge.UserInfo{
		Nick:        nick,
		Real:        dcuser.AuthName,
		User:        strconv.FormatUint(uint64(dcuser.Id), 10),
		Host:        self.Protocol(),
		DisplayName: dcuser.DisplayName,
		Ghost:       true,
		Me:          dcuser.Id == deltachat.ContactSelf,
		Username:    dcuser.DisplayName,
		TeamID:      self.Protocol(),
	}
}

func (self *DeltaChat) createChannelInfo(chatId deltachat.ChatId, isDM bool, chatName string) *bridge.ChannelInfo {
	return &bridge.ChannelInfo{
		Name:    getChanName(chatId, chatName),
		ID:      strconv.FormatUint(uint64(chatId), 10),
		TeamID:  self.Protocol(),
		DM:      isDM,
		Private: false,
	}
}

func getChanName(chatId deltachat.ChatId, chatName string) string {
	prefix := "#"
	suffix := fmt.Sprintf("|%v", chatId)
	name := sanitizeNick(chatName)
	maxSize := 50 - len(prefix) - len(suffix)
	if len(name) > maxSize {
		name = name[:maxSize-1]
	}

	return strings.ToLower(fmt.Sprintf("%s%s%s", prefix, name, suffix))
}

func sanitizeNick(nick string) string {
	sanitize := func(r rune) rune {
		if strings.ContainsRune("!+%@&#$:'\"?*, ", r) {
			return '-'
		}
		return r
	}
	return strings.Map(sanitize, nick)
}

func replaceEmojisWithSlug(s string) string {
	return gomoji.ReplaceEmojisWithFunc(s, func(em gomoji.Emoji) string {
		return ":" + em.Slug + ":"
	})
}
