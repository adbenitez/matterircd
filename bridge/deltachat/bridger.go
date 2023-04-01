// Bridger interface implementation for DeltaChat bridge
package deltachat

import (
	"strconv"
	"strings"
	"time"

	"github.com/deltachat/deltachat-rpc-client-go/deltachat"
	"github.com/deltachat/deltaircd/bridge"
)

func (self *DeltaChat) GetMe() *bridge.UserInfo {
	contact, _ := self.account.Me().Snapshot()
	return self.getUserInfo(contact)
}

func (self *DeltaChat) Connected() bool {
	return self.connected
}

func (self *DeltaChat) Logout() error {
	err := self.account.StopIO()
	if err != nil {
		logger.Error("logout failed", err)
		return err
	}
	logger.Info("logout succeeded")

	self.eventChan <- &bridge.Event{
		Type: "logout",
		Data: &bridge.LogoutEvent{},
	}

	self.connected = false
	return nil
}

func (self *DeltaChat) Protocol() string {
	return "deltachat"
}

func (self *DeltaChat) GetChannels() []*bridge.ChannelInfo {
	var channels []*bridge.ChannelInfo
	chatlistItems, _ := self.account.ChatListItems()
	logger.Debugf("Chatlist has %v items", len(chatlistItems))
	count := 0
	for _, item := range chatlistItems {
		isDM := item.DmChatContact != 0
		if item.Error != "" || !item.IsSelfInGroup || isDM {
			continue
		}
		channel := self.createChannelInfo(item.Id, isDM, item.Name)
		channels = append(channels, channel)
		count++
	}
	return channels
}

func (self *DeltaChat) GetChannel(channelID string) (*bridge.ChannelInfo, error) {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return nil, err
	}
	chat := deltachat.Chat{self.account, id}
	snapshot, err := chat.BasicSnapshot()
	if err != nil {
		return nil, err
	}
	isDM := snapshot.ChatType == deltachat.CHAT_TYPE_SINGLE
	return self.createChannelInfo(chat.Id, isDM, snapshot.Name), nil
}

func (self *DeltaChat) GetChannelName(channelID string) string {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return channelID
	}
	chat := deltachat.Chat{self.account, id}
	snapshot, err := chat.BasicSnapshot()
	if err != nil {
		return channelID
	}
	return getChanName(chat.Id, snapshot.Name)
}

func (self *DeltaChat) GetChannelUsers(channelID string) ([]*bridge.UserInfo, error) {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return nil, err
	}
	chat := deltachat.Chat{self.account, id}
	snapshot, err := chat.FullSnapshot()
	if err != nil {
		return nil, err
	}
	users := make([]*bridge.UserInfo, len(snapshot.Contacts))
	for i := range snapshot.Contacts {
		users[i] = self.getUserInfo(snapshot.Contacts[i])
	}
	if snapshot.ChatType == deltachat.CHAT_TYPE_SINGLE {
		me, _ := self.account.Me().Snapshot()
		users = append(users, self.getUserInfo(me))
	}
	return users, nil
}

func (self *DeltaChat) List() (map[string]string, error) {
	channelInfo := make(map[string]string)
	chatlistItems, _ := self.account.ChatListItems()
	for _, item := range chatlistItems {
		if item.Error != "" || item.DmChatContact != 0 {
			continue
		}
		channelInfo[getChanName(item.Id, item.Name)] = ":" + item.Name
	}

	return channelInfo, nil
}

func (self *DeltaChat) Join(channelName string) (string, string, error) {
	parts := strings.Split(channelName, "|")
	channelID := strings.TrimSpace(parts[len(parts)-1])
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return "", "", err
	}
	chat := deltachat.Chat{self.account, id}
	snapshot, err := chat.BasicSnapshot()
	if err != nil {
		return "", "", err
	}
	return channelID, snapshot.Name, nil
}

func (self *DeltaChat) Topic(channelID string) string {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return ""
	}
	chat := deltachat.Chat{self.account, id}
	snapshot, err := chat.BasicSnapshot()
	if err != nil {
		return ""
	}
	return snapshot.Name
}

func (self *DeltaChat) SetTopic(channelID, text string) error {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return err
	}
	chat := deltachat.Chat{self.account, id}
	return chat.SetName(text)
}

func (self *DeltaChat) GetUsers() []*bridge.UserInfo {
	contacts, _ := self.account.Contacts()
	users := make([]*bridge.UserInfo, len(contacts))
	for i := range contacts {
		snapshot, _ := contacts[i].Snapshot()
		users[i] = self.getUserInfo(snapshot)
	}
	return users
}

func (self *DeltaChat) MsgUser(userID, text string) (string, error) {
	id, err := strconv.ParseUint(userID, 10, 0)
	if err != nil {
		return "", err
	}
	contact := deltachat.Contact{self.account, id}
	chat, err := contact.CreateChat()
	if err != nil {
		return "", err
	}
	msg, err := chat.SendText(text)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(msg.Id, 10), nil
}

func (self *DeltaChat) MsgChannel(channelID, text string) (string, error) {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return "", err
	}
	chat := deltachat.Chat{self.account, id}
	msg, err := chat.SendText(text)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(msg.Id, 10), nil
}

func (self *DeltaChat) StatusUser(userID string) (string, error) {
	id, err := strconv.ParseUint(userID, 10, 0)
	if err != nil {
		return "", err
	}
	contact := deltachat.Contact{self.account, id}
	snapshot, err := contact.Snapshot()
	if err != nil {
		return "", err
	}
	var status string
	if snapshot.WasSeenRecently {
		status = "online"
	} else if snapshot.LastSeen == 0 {
		status = "Last Seen: Never"
	} else {
		lastSeen := time.Unix(int64(snapshot.LastSeen), 0)
		status = "Last Seen: " + lastSeen.Format(time.RFC1123)
	}
	return status, nil
}

func (self *DeltaChat) Part(channelID string) error {
	id, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return err
	}
	chat := deltachat.Chat{self.account, id}
	return chat.Leave()
}

func (self *DeltaChat) Nick(name string) error {
	return self.account.SetConfig("displayname", name)
}

func (self *DeltaChat) Invite(channelID, userID string) error {
	chatId, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return err
	}
	contactId, err := strconv.ParseUint(userID, 10, 0)
	if err != nil {
		return err
	}
	chat := deltachat.Chat{self.account, chatId}
	contact := deltachat.Contact{self.account, contactId}
	return chat.AddContact(&contact)
}

func (self *DeltaChat) Kick(channelID, userID string) error {
	chatId, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return err
	}
	contactId, err := strconv.ParseUint(userID, 10, 0)
	if err != nil {
		return err
	}
	chat := deltachat.Chat{self.account, chatId}
	contact := deltachat.Contact{self.account, contactId}
	return chat.RemoveContact(&contact)
}

func (self *DeltaChat) GetChannelID(name, teamID string) string {
	parts := strings.Split(name, "|")
	channelID := strings.TrimSpace(parts[len(parts)-1])
	_, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return name
	}
	return channelID
}

func (self *DeltaChat) GetPosts(channelID string, limit int) interface{} {
	if limit < 1 {
		return nil
	}
	//parts := strings.Split(name, "|")
	//channelID := strings.TrimSpace(parts[len(parts)-1])
	chatId, err := strconv.ParseUint(channelID, 10, 0)
	if err != nil {
		return nil
	}

	chat := deltachat.Chat{self.account, chatId}
	msgs, err := chat.Messages(false, false)
	if err != nil {
		return nil
	}
	maxIndex := len(msgs)
	if maxIndex < limit {
		limit = maxIndex
	}
	return msgs[maxIndex-limit:] // FIXME: fix scrollback()
}

func (self *DeltaChat) SearchUsers(query string) ([]*bridge.UserInfo, error) {
	contacts, err := self.account.QueryContacts(query, 0)
	var users []*bridge.UserInfo
	if err != nil {
		return users, err
	}
	users = make([]*bridge.UserInfo, len(contacts))
	for i := range contacts {
		contact, _ := contacts[i].Snapshot()
		users[i] = self.getUserInfo(contact)
	}
	return users, nil
}

func (self *DeltaChat) GetTeamName(teamID string) string {
	return ""
}

func (self *DeltaChat) StatusUsers() (map[string]string, error) {
	return make(map[string]string), nil
}

func (self *DeltaChat) UpdateChannels() error {
	return nil
}

func (self *DeltaChat) MsgChannelThread(channelID, parentID, text string) (string, error) {
	return "", nil
}

func (self *DeltaChat) MsgUserThread(userID, parentID, text string) (string, error) {
	return "", nil
}

func (self *DeltaChat) ModifyPost(msgID, text string) error {
	return nil
}

func (self *DeltaChat) GetFileLinks(fileIDs []string) []string {
	return []string{}
}

// set "online" | "away" status
func (self *DeltaChat) SetStatus(status string) error {
	return nil
}

// Not implemented yet

func (self *DeltaChat) AddReaction(msgID, emoji string) error    { return nil }
func (self *DeltaChat) RemoveReaction(msgID, emoji string) error { return nil }

func (self *DeltaChat) GetPostsSince(channelID string, since int64) interface{} { return nil }

func (self *DeltaChat) SearchPosts(search string) interface{}                { return nil }
func (self *DeltaChat) GetUser(userID string) *bridge.UserInfo               { return nil }
func (self *DeltaChat) GetUserByUsername(username string) *bridge.UserInfo   { return nil }
