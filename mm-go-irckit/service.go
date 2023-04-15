package irckit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/deltachat/deltachat-rpc-client-go/deltachat"
	"github.com/deltachat/deltaircd/bridge"
)

type CommandHandler interface {
	handle(u *User, c *Command, args []string, service string)
}

// nolint:structcheck
type Command struct {
	handler   func(u *User, toUser *User, args []string, service string)
	minParams int
	maxParams int
	login     bool
}

func logout(u *User, toUser *User, args []string, service string) {
	if u.inprogress {
		u.MsgUser(toUser, "login or logout in progress. Please wait")
		return
	}
	u.br.Logout()
	u.logoutFrom(u.br.Protocol())
}

func login(u *User, toUser *User, args []string, service string) {
	if u.inprogress {
		u.MsgUser(toUser, "login or logout in progress. Please wait")
		return
	}

	switch len(args) {
	case 2:
		u.Credentials = bridge.Credentials{
			Login: args[0],
			Pass:  args[1],
		}
	case 1:
		u.Credentials = bridge.Credentials{Login: args[0]}
	case 0:
	default:
		u.MsgUser(toUser, "need LOGIN <email> [pass]")
		return
	}

	u.inprogress = true
	defer func() { u.inprogress = false }()

	if err := u.loginTo("deltachat"); err != nil {
		u.MsgUser(toUser, err.Error())
		return
	}

	u.MsgUser(toUser, "login OK")
}

//nolint:cyclop
func search(u *User, toUser *User, args []string, service string) {
	posts, ok := u.br.SearchPosts(strings.Join(args, " ")).([]*deltachat.MsgSearchResult)

	if !ok || posts == nil || len(posts) == 0 {
		u.MsgUser(toUser, "no results")
		return
	}

	for i := len(posts) - 1; i >= 0; i-- {
		post := posts[i]
		timestamp := post.Timestamp.Format("January 02, 2006 15:04")
		header := "<" + post.AuthorName + "> " + timestamp
		u.MsgUser(toUser, header)
		u.MsgUser(toUser, strings.Repeat("=", len(header)))

		for _, post := range strings.Split(post.Message, "\n") {
			if post != "" {
				u.MsgUser(toUser, post)
			}
		}
		u.MsgUser(toUser, "")
		u.MsgUser(toUser, "")
	}
}

func searchUsers(u *User, toUser *User, args []string, service string) {
	users, err := u.br.SearchUsers(strings.Join(args, " "))
	if err != nil {
		u.MsgUser(toUser, fmt.Sprint("Error", err.Error()))
		return
	}

	for _, user := range users {
		u.MsgUser(toUser, fmt.Sprint(user.Nick, user.FirstName, user.LastName))
	}
}

//nolint:funlen,gocognit,gocyclo,cyclop
func scrollback(u *User, toUser *User, args []string, service string) {
	if len(args) != 2 {
		u.MsgUser(toUser, "need SCROLLBACK (#<channel>|<user>) <lines>")
		u.MsgUser(toUser, "e.g. SCROLLBACK #bugs 10 (show last 10 lines from #bugs)")
		return
	}

	limit, err := strconv.Atoi(args[1])
	if err != nil {
		u.MsgUser(toUser, "need SCROLLBACK (#<channel>|<user>) <lines>")
		u.MsgUser(toUser, "e.g. SCROLLBACK #bugs 10 (show last 10 lines from #bugs)")
		return
	}

	var channelID string
	var spoof func(string, string, ...int)
	scrollbackUser, exists := u.Srv.HasUser(args[0])

	switch {
	case strings.HasPrefix(args[0], "#"):
		channelName := strings.ReplaceAll(args[0], "#", "")
		channelID = u.br.GetChannelID(channelName, u.br.GetMe().TeamID)
		spoof = u.Srv.Channel(channelID).SpoofMessage
	case exists && scrollbackUser.Ghost:
		channelID = u.br.GetUserChannelID(scrollbackUser.User, u.br.GetMe().TeamID)
	default:
		u.MsgUser(toUser, "need SCROLLBACK (#<channel>|<user>) <lines>")
		u.MsgUser(toUser, "e.g. SCROLLBACK #bugs 10 (show last 10 lines from #bugs)")
		return
	}

	posts, ok := u.br.GetPosts(channelID, limit).([]*deltachat.Message)
	if !ok || posts == nil || len(posts) == 0 {
		u.MsgUser(toUser, "no results")
		return
	}

	for i := range posts {
		msgData, err := posts[i].Snapshot()
		if err != nil {
			continue
		}
		ts := msgData.Timestamp

		user := u.br.GetUser(msgData.Sender)
		nick := user.Nick
		if msgData.IsInfo {
			nick = "system"
		}

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

		for _, post := range strings.Split(text, "\n") {
			switch { // nolint:dupl
			case u.v.GetBool(u.br.Protocol()+".prefixcontext") && strings.HasPrefix(args[0], "#") && nick != "system":
				quotedId := ""
				if msgData.Quote != nil && msgData.Quote.MessageId != 0 {
					quotedId = strconv.FormatUint(uint64(msgData.Quote.MessageId), 10)
				}
				threadMsgID := u.prefixContext("", strconv.FormatUint(uint64(msgData.Id), 10), quotedId, "")
				scrollbackMsg := u.formatContextMessage(ts.Format("2006-01-02 15:04"), threadMsgID, post)
				spoof(nick, scrollbackMsg)
			case strings.HasPrefix(args[0], "#"):
				scrollbackMsg := "[" + ts.Format("2006-01-02 15:04") + "] " + post
				spoof(nick, scrollbackMsg)
			case u.v.GetBool(u.br.Protocol()+".prefixcontext"):
				quotedId := ""
				if msgData.Quote != nil && msgData.Quote.MessageId != 0 {
					quotedId = strconv.FormatUint(uint64(msgData.Quote.MessageId), 10)
				}
				threadMsgID := u.prefixContext("", strconv.FormatUint(uint64(msgData.Id), 10), quotedId, "")
				scrollbackMsg := u.formatContextMessage(ts.Format("2006-01-02 15:04"), threadMsgID, post)
				u.MsgSpoofUser(scrollbackUser, nick, scrollbackMsg)
			default:
				scrollbackMsg := "[" + ts.Format("2006-01-02 15:04") + "]" + " <" + nick + "> " + post
				u.MsgSpoofUser(scrollbackUser, nick, scrollbackMsg)
			}
		}
	}
}

var cmds = map[string]Command{
	"logout":      {handler: logout, login: true, minParams: 0, maxParams: 0},
	"login":       {handler: login, minParams: 0, maxParams: 2},
	"search":      {handler: search, login: true, minParams: 1, maxParams: -1},
	"searchusers": {handler: searchUsers, login: true, minParams: 1, maxParams: -1},
	"scrollback":  {handler: scrollback, login: true, minParams: 2, maxParams: 2},
}

func (u *User) handleServiceBot(service string, toUser *User, msg string) {
	commands, err := parseCommandString(msg)
	if err != nil {
		u.MsgUser(toUser, fmt.Sprintf("\"%s\" is improperly formatted", msg))
		return
	}

	cmd, ok := cmds[strings.ToLower(commands[0])]
	if !ok {
		keys := make([]string, 0)
		for k := range cmds {
			keys = append(keys, k)
		}
		u.MsgUser(toUser, "possible commands: "+strings.Join(keys, ", "))
		u.MsgUser(toUser, "<command> help for more info")
		return
	}

	if cmd.login {
		if u.br == nil {
			u.MsgUser(toUser, "You're not logged in. Use LOGIN first.")
			return
		}
	}
	/*
		if cmd.minParams > len(commands[1:]) {
			u.MsgUser(toUser, fmt.Sprintf("%s requires at least %v arguments", commands[0], cmd.minParams))
			return
		}
	*/
	if cmd.maxParams > -1 && len(commands[1:]) > cmd.maxParams {
		u.MsgUser(toUser, fmt.Sprintf("%s takes at most %v arguments", commands[0], cmd.maxParams))
		return
	}

	cmd.handler(u, toUser, commands[1:], service)
}

func parseCommandString(line string) ([]string, error) {
	args := []string{}
	buf := ""
	var escaped, doubleQuoted, singleQuoted bool

	got := false

	for _, r := range line {
		// If the string is escaped
		if escaped {
			buf += string(r)
			escaped = false
			continue
		}

		// If "\"
		if r == '\\' {
			if singleQuoted {
				buf += string(r)
			} else {
				escaped = true
			}
			continue
		}

		// If it is whitespace
		if unicode.IsSpace(r) {
			if singleQuoted || doubleQuoted {
				buf += string(r)
			} else if got {
				args = append(args, buf)
				buf = ""
				got = false
			}
			continue
		}
		// If Quoted
		switch r {
		case '"':
			if !singleQuoted {
				doubleQuoted = !doubleQuoted
				continue
			}
		case '\'':
			if !doubleQuoted {
				singleQuoted = !singleQuoted
				continue
			}
		}
		got = true
		buf += string(r)
	}

	if got {
		args = append(args, buf)
	}

	if escaped || singleQuoted || doubleQuoted {
		return nil, errors.New("invalid command line string")
	}

	return args, nil
}
