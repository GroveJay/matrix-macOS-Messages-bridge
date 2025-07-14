package macos

type GroupActionType int

const (
	GroupActionAddUser    GroupActionType = 0
	GroupActionRemoveUser GroupActionType = 1

	GroupActionSetAvatar    GroupActionType = 1
	GroupActionRemoveAvatar GroupActionType = 2
)

type ItemType int

const (
	ItemTypeMessage ItemType = iota
	ItemTypeMember
	ItemTypeName
	ItemTypeAvatar
	ItemTypeLocationSharing
	ItemTypeShareplay ItemType = 6
	ItemTypeError     ItemType = -100
)

const CHAT_HANDLES_IDS_SEPARATOR = ";"
const CHAT_HANDLES_IDS_COLUMN_NAME = "chat_handles_ids"

const chatHandleJoin = `
	JOIN chat_handle_join ON chat_handle_join.chat_id = inner_chat.ROWID
	JOIN handle ON chat_handle_join.handle_id = handle.ROWID
`

const chatHandlesIDsSubquery = `
(
	SELECT group_concat(handle.id, "` + CHAT_HANDLES_IDS_SEPARATOR + `")
	FROM chat as inner_chat` + chatHandleJoin +
	`	WHERE inner_chat.guid=chat.guid
) ` + CHAT_HANDLES_IDS_COLUMN_NAME

const baseMessagesQuery = `
SELECT message.*,
chat.guid, ` + chatHandlesIDsSubquery + `,
COALESCE(handle_on_handle_id.id, ''), COALESCE(handle_on_other_handle.id, '')
FROM message
LEFT JOIN chat_message_join    ON chat_message_join.message_id = message.ROWID
LEFT JOIN chat                 ON chat_message_join.chat_id = chat.ROWID
LEFT JOIN handle handle_on_handle_id ON message.handle_id = handle_on_handle_id.ROWID
LEFT JOIN handle handle_on_other_handle ON message.other_handle = handle_on_other_handle.ROWID
`

const GroupMemberQuery = `
SELECT handle.id, handle.country FROM chat as inner_chat` + chatHandleJoin + `WHERE inner_chat.guid=$1`

const ChatQuery = `
SELECT COALESCE(display_name, '')
FROM chat
WHERE guid=$1
`

const GroupActionQuery = `
SELECT COALESCE(attachment.filename, ''), COALESCE(attachment.mime_type, ''), attachment.transfer_name
FROM message
JOIN chat_message_join ON chat_message_join.message_id = message.ROWID
JOIN chat              ON chat_message_join.chat_id = chat.ROWID
LEFT JOIN message_attachment_join ON message_attachment_join.message_id = message.ROWID
LEFT JOIN attachment              ON message_attachment_join.attachment_id = attachment.ROWID
WHERE message.item_type=$1 AND message.group_action_type=$2 AND chat.guid=$3
ORDER BY message.date DESC LIMIT 1
`

const MaxMessagesTimeQuery = `
SELECT MAX(MAX(date), MAX(date_edited), MAX(date_retracted)) FROM message
`

const NewMessagesQuery = baseMessagesQuery + `
WHERE message.ROWID > $1
ORDER BY message.date ASC
`

const MessagesNewerThanQuery = baseMessagesQuery + `
WHERE message.date > $1 OR message.date_edited > $1 OR message.date_retracted > $1
ORDER BY COALESCE(NULLIF(message.date_retracted, 0), COALESCE(NULLIF(message.date_edited, 0), message.date)) ASC
`

const MessagesBetweenQuery = baseMessagesQuery + `
WHERE message.ROWID > $1 AND message.ROWID < $2
ORDER BY message.date ASC
`

const MessagesByGUID = baseMessagesQuery + `
WHERE message.guid == $1
ORDER BY message.date ASC
`

const NewRecieptsQuery = `
SELECT chat.guid, ` + chatHandlesIDsSubquery + `,
message.guid, message.is_from_me, message.date_read
FROM message
JOIN chat_message_join ON chat_message_join.message_id = message.ROWID
JOIN chat              ON chat_message_join.chat_id = chat.ROWID
WHERE date_read>$1 AND is_read=1
`

const AttachmentsQuery = `
SELECT attachment.*
FROM attachment
JOIN message_attachment_join ON message_attachment_join.attachment_id = attachment.ROWID
WHERE message_attachment_join.message_id = $1
ORDER BY ROWID
`

const ContactsQuery = `
select r.ZUNIQUEID, COALESCE(r.ZFIRSTNAME, ''), COALESCE(r.ZLASTNAME, ''), COALESCE(r.ZNICKNAME, ''), COALESCE(p.ZFULLNUMBER, ''), COALESCE(e.ZADDRESSNORMALIZED, '')
from ZABCDRECORD as r 
LEFT JOIN ZABCDPHONENUMBER as p
ON p.ZOWNER=r.Z_PK
LEFT JOIN ZABCDEMAILADDRESS as e
ON e.ZOWNER=r.Z_PK WHERE
(
	e.ZADDRESSNORMALIZED != "" OR P.ZFULLNUMBER != ""
) AND
(
	r.ZFIRSTNAME != "" OR r.ZLASTNAME != "" OR r.ZNICKNAME != ""
);
`

const SendMessageToChatGUID = `
on run {chatGUID, message}
	tell application "Messages"
		send message to chat id chatGUID
	end tell
end run
`

const GetChatGUIDFromHandlesIDs = `
on run {chat_handles_ids}
	tell application "Messages"
		set cs to get every chat
		set AppleScript's text item delimiters to "` + CHAT_HANDLES_IDS_SEPARATOR + `"
		set handles_ids to every text item of chat_handles_ids
		set handles_count to count of handles_ids
		set chat_guid to ""
		repeat with c in cs
			set ps to get participants of c
			set participants_count to count of ps

			try
				if participants_count is not equal to handles_count then
					error 0
				end if
				repeat with p in ps
					set p_id to handle of p
					if handles_ids does not contain p_id then
						error 0
					end if
				end repeat
				set chat_guid to id of c
				exit repeat
			end try
		end repeat
		copy chat_guid to stdout
	end tell
end run

`

const CheckMessagesRunning = `
set messages_app_id to get id of application "Messages"
set messages_app to application id messages_app_id
if messages_app is not running then
	error "Messages not running"
end if
`

const CheckContactsRunning = `
set contacts_app_id to get id of application "Contacts"
set contacts_app to application id contacts_app_id
if contacts_app is not running then
	error "Contacts not running"
end if
`

const GetContactVCard = `
on run {contactID}
	tell application "Contacts"
		set person_with_id to get first person whose id = contactID
		if image of person_with_id is not missing value then
			get vcard of person_with_id
		end if
	end tell
end run
`

const GetOwnContactFirstPhone = `
tell application "Contacts"	
	copy value of first phone of my card as string to stdout
end tell
`
