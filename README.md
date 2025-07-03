# matrix-macOS-Message-bridge
A Matrix macOS Messages puppeting bridge.

## SENDING MESSAGES IS NOT WORKING YET

## Prior Art

### Bridges

* [mautrix-imessage](https://github.com/mautrix/imessage) - Multi-purpose bridge for:
  * macOS
  * macOS without SIP (deprecated due to Barcelona deprecation)
  * iOS (deprecated)
  * Android (deprecated in favor of mautrix-gmessages)
* [beeper-imessage](https://github.com/beeper/imessage) - Deprecated/Archived

### NSAttributedString

Kinda crazy we're still stuck with stuff from NextStep?

Primary implementations of decoding NSAttributedString/typedstream/streamtyped from [python-typedstream](https://github.com/dgelessus/python-typedstream/tree/main/src/typedstream) and [ReagentX/immessage-exporter](https://github.com/ReagentX/imessage-exporter) and further documented in an extensive write-up [here](https://chrissardegna.com/blog/reverse-engineering-apples-typedstream-format/)

Other implementations:

* [BlueBubbles/node-typedstream](https://github.com/BlueBubblesApp/node-typedstream/blob/master/src/stream.ts) (probably entirely workable since they're doing what we're doing)
* meowUnsafeDecodeAttributedString from mautrix-imessage (calls out to ObjectiveC and throws away a lot of data in the macOS code path)
* [yakuter/nsattrparser](https://github.com/yakuter/nsattrparser/blob/main/nsattrparser.go) (surface level string grab)

## Architectural Decisions

The three main reasons for this bridge were:

1. All but one existing bridge is deprecated
2. The single operable bridge (`mautrix-imessage` in macOS mode) has some sharp edges
3. `mautrix-imessage` could be upgraded to the shiny "new" bridgev2

The intention is to create a bare-minimum replacement for the `mautrix-imessage` macOS bridge using bridgev2 and attempt to remove these sharp edges:

1. Easier installation as a LaunchAgent or LaunchDaemon (Daemon may be impossible due to permissions issues)
2. Improve startup robustness by addressing or surfacing issues:
   1. Detect if Messages needs to be launched or restarted
   2. Attempt to ensure TCC (Transparency, Consent and Control) dialogs pops up if required by making the bridge a full-blown macOS application
   3. Check all external dependencies work before starting
3. Replace brittle Contacts support if possible
4. Remove requirement for `Full Disk Access`

## Documentation

This bridge can't be run inside a docker container as it has to directly monitor the Messages sqlite database and access the Contacts sqlite databases.
Since it also doesn't have any executable dependencies, it's fastest to build it on the target machine.

1. Build the bridge
```bash
./build.sh
```
This builds and copies the bridge to it's required location and name so it can become a full macOS Application later.

2. Change into the Application folder
```bash
cd ./Matrix-MacOS-Messages-Bridge.app/Contents/MacOS/
```
3. Run the bridge with arguments to generate a config file
```bash
./Matrix-MacOS-Messages-Bridge -c ./config.yaml -e
```
4. Customize the config. In particular you'll want to pay attention to:
- bridge.permissions
  - It's likely best to give just your own user the `admin` permission
- database.uri
- homeserver.address (address the bridge uses to connect to homeserver)
- homeserver.domain (domain of the homeserver)
- appservice.address (Address the homeserver will use to connect to the bridge)
  - If the homeserver is in docker, use a special address like `http://host.docker.internal:29441`
- appservice.public_address
- appservice.hostname: 127.0.0.1
- double_puppet.secrets
- encryption.allow: true
5. Generate the bridge registration
```bash
./Matrix-MacOS-Messages-Bridge -g -c ./config.yaml -r ./registration.yaml
```
6. Add the bridge registration to the homeserver, moving the `registration.yaml` if necessary.
7. Create the user and database in the `postgres` instance you're using for other bridges and/or the homeserver.
8. Copy the Application folder into your `/Applications` folder
9. Copy the LaunchAgent plist to your `~/Library/LaunchAgents` folder
10. Load the LaunchAgent with `launchctl`
```bash
launchctl load ~/Library/LaunchAgents/matrix-macOS-Messages-bridge.plist
```
To remove the LaunchAgent run:
```bash
launchctl bootout matrix-macOS-Messages-bridge
```
11. Check the error logs at `/usr/local/var/log/matrix-macOS-Messages-bridge.error.log` and regular logs at `/usr/local/var/log/matrix-macOS-Messages-bridge.log`
12. Message the bot in your homeserver at `@Messagesbot:<server_url>`
13. Test the bot is repsonsive by messaging `help`
14. Use `login user-id` to confirm your user ID (phone number) is discovered correctly
15. Complete the login process and check the logs again to see that your Messages database is being watched for updates.