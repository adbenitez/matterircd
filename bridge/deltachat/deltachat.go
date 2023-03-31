package deltachat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deltachat/deltachat-rpc-client-go/deltachat"
	"github.com/deltachat/deltaircd/bridge"
	logger "github.com/sirupsen/logrus"
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

func New(cfg *viper.Viper, cred bridge.Credentials, eventChan chan<- *bridge.Event, onConnect func()) (bridge.Bridger, error) {
	dc := &DeltaChat{
		credentials: cred,
		eventChan:   eventChan,
		cfg:         cfg,
		onConnect:   onConnect,
	}

	logger.SetFormatter(&logger.TextFormatter{FullTimestamp: true})
	if cfg.GetBool("debug") {
		logger.SetLevel(logger.DebugLevel)
	}

	if cfg.GetBool("trace") {
		logger.SetLevel(logger.TraceLevel)
	}

	if err := dc.loginToDeltaChat(); err != nil {
		return nil, err
	}
	return dc, nil
}

func (self *DeltaChat) loginToDeltaChat() error {
	rpc := deltachat.NewRpcIO()
	rpc.AccountsDir = self.cfg.GetString(self.Protocol() + ".accounts")
	rpc.Start()

	manager := deltachat.AccountManager{rpc}
	accounts, _ := manager.Accounts()

	if self.credentials.Login != "" {
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

	if self.credentials.Pass != "" {
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

func (self *DeltaChat) handleEvent(event *deltachat.Event) {
	switch evtype := event.Type; evtype {
	case deltachat.EVENT_INFO:
		logger.Debug("INFO:", event.Msg)
	case deltachat.EVENT_WARNING:
		logger.Debug("WARNING:", event.Msg)
	case deltachat.EVENT_ERROR:
		logger.Debug("ERROR:", event.Msg)
	case deltachat.EVENT_INCOMING_MSG:
		chat := &deltachat.Chat{self.account, event.ChatId}
		chat.SetMuteDuration(0) // TODO: remove this when there is a way to get fresh messages from muted chats
		self.processMessages()
	case deltachat.EVENT_MSGS_CHANGED:
		if event.MsgId != 0 {
			msg := &deltachat.Message{self.account, event.MsgId}
			msgData, err := msg.Snapshot()
			if err == nil && msgData.IsInfo {
				self.processInfoMsg(msgData)
			}

		}
	}
}

func (self *DeltaChat) processInfoMsg(msgData *deltachat.MsgSnapshot) bool {
	switch msgData.SystemMessageType {
	case deltachat.SYSMSG_TYPE_MEMBER_ADDED_TO_GROUP:
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
				ChannelID: strconv.FormatUint(msgData.ChatId, 10),
			},
		}
		self.eventChan <- event
		return true
	case deltachat.SYSMSG_TYPE_MEMBER_REMOVED_FROM_GROUP:
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
				ChannelID: strconv.FormatUint(msgData.ChatId, 10),
			},
		}
		self.eventChan <- event
		return true
	case deltachat.SYSMSG_TYPE_GROUP_NAME_CHANGED:
		if msgData.FromId == deltachat.CONTACT_SELF {
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
				ChannelID: strconv.FormatUint(msgData.ChatId, 10),
				UserID:    strconv.FormatUint(msgData.FromId, 10),
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

func (self *DeltaChat) processMsg(msg *deltachat.MsgSnapshot) {
	text := msg.Text
	if msg.File != "" {
		if text != "" {
			text = msg.File + " - " + text
		} else {
			text = msg.File
		}
	}
	logger.Debugf("Processing message (id=%v)", msg.Id)

	ghost := self.getUserInfo(msg.Sender)

	chat := deltachat.Chat{self.account, msg.ChatId}
	chatData, _ := chat.BasicSnapshot()
	channelID := strconv.FormatUint(chat.Id, 10)
	if chatData.ChatType == deltachat.CHAT_TYPE_SINGLE {
		self.sendDirectMessage(ghost, ghost, channelID, msg.Text)
	} else {
		self.sendPublicMessage(ghost, channelID, msg.Text)
	}
}

func (self *DeltaChat) sendDirectMessage(sender, receiver *bridge.UserInfo, channelID, text string) {
	for _, msg := range strings.Split(text, "\n") {
		event := &bridge.Event{
			Type: "direct_message",
			Data: &bridge.DirectMessageEvent{
				Text:      msg,
				Sender:    sender,
				Receiver:  receiver,
				ChannelID: channelID,
			},
		}
		self.eventChan <- event
	}
}

func (self *DeltaChat) sendPublicMessage(ghost *bridge.UserInfo, channelID, text string) {
	for _, msg := range strings.Split(text, "\n") {
		event := &bridge.Event{
			Type: "channel_message",
			Data: &bridge.ChannelMessageEvent{
				Text:      msg,
				ChannelID: channelID,
				Sender:    ghost,
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
		User:        strconv.FormatUint(dcuser.Id, 10),
		Host:        self.Protocol(),
		DisplayName: dcuser.DisplayName,
		Ghost:       true,
		Me:          dcuser.Id == deltachat.CONTACT_SELF,
		Username:    dcuser.DisplayName,
		TeamID:      self.Protocol(),
	}
}

func (self *DeltaChat) createChannelInfo(chatId uint64, isDM bool, chatName string) *bridge.ChannelInfo {
	return &bridge.ChannelInfo{
		Name:    getChanName(chatId, chatName),
		ID:      strconv.FormatUint(chatId, 10),
		TeamID:  self.Protocol(),
		DM:      isDM,
		Private: false,
	}
}

func getChanName(chatId uint64, chatName string) string {
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
