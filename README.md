<img height="180" width="100%" src="https://github.com/deltachat/deltaircd/raw/master/images/logo.svg" alt="deltaircd">

<p align="center">Minimal IRC server which integrates with <a href="https://delta.chat">Delta Chat</a></p>

## Features

- support direct messages / private channels
- auto-join/leave to same channels as on Delta Chat
- reconnects with backoff on Delta Chat restarts
- support multiple users
- support channel/direct message backlog (messages when you're disconnected from IRC/Delta Chat)
- WHOIS, WHO, JOIN, LEAVE, NICK, LIST, ISON, PRIVMSG, MODE, TOPIC, LUSERS, AWAY, KICK, INVITE support
- support TLS (ssl)
- support unix sockets
- &users channel that contains all your contacts for easy messaging
- support for including/excluding channels from showing up in IRC
- support multiline pasting
- TODO: prefixcontext option (see <https://github.com/deltachat/deltaircd/blob/master/prefixcontext.md>)
  - TODO: replies support
  - TODO: reactions support
  - TODO: delete messages
- TODO: search messages (/msg deltachat search query)
- TODO: scrollback support (/msg deltachat scrollback #channel limit)

## Installing dependencies

To use deltairc first make sure you have `deltachat-rpc-server` program installed in your
`PATH`, for more info check:
https://github.com/deltachat/deltachat-core-rust/tree/master/deltachat-rpc-server

## Building

Go 1.17+ is required

```bash
go install github.com/deltachat/deltaircd
```

You should now have `deltaircd` binary in the bin directory:

```bash
$ ls ~/go/bin/
deltaircd
```

## Config file

See [deltaircd.toml.example](https://github.com/deltachat/deltaircd/blob/master/deltaircd.toml.example)
Run with `deltaircd --conf deltaircd.toml`

## Usage

```
Usage of ./deltaircd:
      --bind string      interface:port to bind to, or a path to bind to a Unix socket. (default "127.0.0.1:6667")
      --conf string      config file (default "deltaircd.toml")
      --debug            enable debug logging
      --tlsbind string   interface:port to bind to. (e.g 127.0.0.1:6697)
      --tlsdir string    directory to look for key.pem and cert.pem. (default ".")
      --version          show version
```

deltaircd will listen by default on localhost port 6667.
Connect with your favorite IRC client to localhost:6667

For TLS support you'll need to generate certificates.
You can use this program [generate_cert.go](https://golang.org/src/crypto/tls/generate_cert.go) to
generate key.pem and cert.pem

### Delta Chat user commands

Configure a new account with email/pass and login into it

```
/msg deltachat login <email> <password>
```

Login into existing previously configured accout

```
/msg deltachat login <email>
```

## Credits

deltaircd is a port of [matterircd](https://github.com/42wim/matterircd) for Delta Chat.
