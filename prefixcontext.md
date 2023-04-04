# prefixcontext

When enabling this you'll get a hex number between [000] and [fff] prefixed to each message.
Every channel/direct message will have a seperate counter.

This way you can see what operation has happened on which message.

## view reactions

Now you can also see those reactions, in future versions this may become unicode.

```irc
01:20 <@wim> [005] test
01:20 <@wim> [006] another message
01:20 <@wim> [007] something else
01:20 <@wim> [006] reactions changed (:sunglasses: 1)
01:21 <@wim> [005] reactions changed (:rofl: 1)
```

## view replies

You can also see who replied to what message
[replynumber->quotenumber]

```irc
19:58 <@wim> [001] normal message
19:58 <@wim> [002] another one
19:58 <@wim> [003->001] in a reply
19:58 <@wim> [004->001] another reply to the same message
19:58 <@wim> [005] normal message
```

## reply to messages

With `@@number` you can reply to a message and it'll be a quote-reply in Delta Chat

```irc
21:55 <wim> [001] abc
21:56 <wim> [002] def
21:56 <wim> [003->002] lala
21:56 <wimtest> @@001 xyz
```

## delete messages


To delete a message just send `s/number/`

You'll have to calculate the number for your own messages yourself, in the example below it's `003`

```irc
23:25 <@wim> [001] hi
23:25 <@wim> [002] something
23:25 < wimirc> hello how are you?
23:25 < wimirc> s/003/
```

You can also delete the last message you sent.

```irc
23:25 <@wim> [001] hi
23:25 <@wim> [002] something
23:25 < wimirc> hllo how are you
23:25 < wimirc> s/!!/
23:25 <@wim> [004] fine
23:25 < wimirc> good
23:25 < wimirc> s//
```

Or reply the last message you sent.

```irc
23:25 <@wim> [001] hi
23:25 <@wim> [002] something
23:25 < wimirc> hello
23:25 < wimirc> @@!! how are you?
23:25 <@wim> [005->004] good
```

## add/remove reactions to messages

To add a reaction to a message, just use +:reaction: as follows:

```irc
23:25 <@wim> [001] hi
23:25 < wimirc> hello how are you
23:25 <@wim> [003] fine
23:25 < wimirc> @@003 +:thumbsup:
23:25 < wimirc> reactions changed (:thumbsup: 1)
```

You can also remove reactions.

```irc
23:25 <@wim> [001] hi
23:25 < wimirc> hello how are you
23:25 <@wim> [003] fine
23:25 < wimirc> @@003 +:thumbsup:
23:25 < wimirc> reactions changed (:thumbsup: 1)
23:25 < wimirc> @@003 -:thumbsup:
23:25 < wimirc> reactions changed (no reactions)
```
