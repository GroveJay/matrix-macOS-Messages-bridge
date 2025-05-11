# matrix-macOS-Message-bridge
A Matrix macOS Messages puppeting bridge.

## Prior Art

### Bridges

* [mautrix-imessage](https://github.com/mautrix/imessage) - Multi-purpose bridge for:
  * macOS
  * macOS without SIP (deprecated due to Barcelona deprecation)
  * iOS (deprecated)
  * Android (deprecated in favor of mautrix-gmessages)
* [beeper-imessage](https://github.com/beeper/imessage) - Deprecated/Archived

### NSAttributedString

Kinda crazy we're still stuck with stuff from NextStep

Primary implementation of decoding NSAttributedString/typedstream/streamtyped from [ReagentX/immessage-exporter](https://github.com/ReagentX/imessage-exporter) and further documented in an extensive write-up [here](https://chrissardegna.com/blog/reverse-engineering-apples-typedstream-format/)

Other implementations:

* [python-typedstream](https://github.com/dgelessus/python-typedstream/tree/main/src/typedstream) (partial implementation, but probably workable)
* [BlueBubbles/node-typedstream](https://github.com/BlueBubblesApp/node-typedstream/blob/master/src/stream.ts) (probably entirely workable since they're doing what we're doing)
* meowUnsafeDecodeAttributedString from mautrix-imessage (calls out to ObjectiveC and throws away a lot of data in the macOS code path)
* [yakuter/nsattrparser](https://github.com/yakuter/nsattrparser/blob/main/nsattrparser.go) (surface level string grab)

## Architectural Decisions

The three main reasons for this bridge were:

1. All but one existing bridge is deprecated
2. The single operable bridge (mautrix-imessage in macOS mode) has some sharp edges
3. mautrix-imessage could be upgraded to the shiny "new" bridgev2

With that in mind, the intention is to create a bare-minimum replacement for mautrix-imessage's macOS bridge using bridgev2 and attempt to remove these sharp edges:

1. Easier installation as either a LaunchAgent or LaunchDaemon (Daemon may be impossible due to permissions issues)
2. Improve bridge startup robustness by addressing or surfacing issues:
   1. Detect if Messages needs to be launched or restarted
   2. Attempt to ensure TCC (Transparency, Consent and Control) dialogs pops up if required by making the bridge a full-blown macOS application if needed
   3. Check all external dependencies work before starting
3. Replace brittle Contacts support if possible
4. Attempt to remove requirement for `Full Disk Access`

## Documentation

**TODO**: Document building and running the bridge as a process and as a LaunchAgent.