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

	ItemTypeError ItemType = -100
)

/*
Old query for posterity:
message.ROWID, message.guid, message.date, COALESCE(message.subject, ''), COALESCE(message.text, ''), message.attributedBody, message.message_summary_info,
chat.guid, COALESCE(sender_handle.id, ''), COALESCE(sender_handle.service, ''), COALESCE(target_handle.id, ''), COALESCE(target_handle.service, ''),
message.is_from_me, message.date_read, message.is_delivered, message.is_sent, message.is_emote, message.is_audio_message, message.date_edited, message.date_retracted,
COALESCE(message.thread_originator_guid, ''), COALESCE(message.thread_originator_part, ''), COALESCE(message.associated_message_guid, ''), message.associated_message_type, COALESCE(message.associated_message_emoji, ''),
message.group_title, message.item_type, message.group_action_type, chat.group_id, COALESCE(message.balloon_bundle_id, '')
*/

const baseMessagesQuery = `
SELECT message.*,
chat.guid, chat.group_id,
COALESCE(sender_handle.id, ''), COALESCE(sender_handle.service, ''),
COALESCE(target_handle.id, ''), COALESCE(target_handle.service, '')
FROM message
JOIN chat_message_join         ON chat_message_join.message_id = message.ROWID
JOIN chat                      ON chat_message_join.chat_id = chat.ROWID
LEFT JOIN handle sender_handle ON message.handle_id = sender_handle.ROWID
LEFT JOIN handle target_handle ON message.other_handle = target_handle.ROWID
`

const GroupMemberQuery = `
SELECT handle.id, handle.country FROM chat
JOIN chat_handle_join ON chat_handle_join.chat_id = chat.ROWID
JOIN handle ON chat_handle_join.handle_id = handle.ROWID
WHERE chat.guid=$1
`

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

const MaxMessagesRowQuery = `
SELECT MAX(ROWID) FROM message
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
ORDER BY COALESCE(message.date_retracted, COALESCE(message.date_edited, message.date)) ASC
`

const MessagesBetweenQuery = baseMessagesQuery + `
WHERE message.ROWID > $1 AND message.ROWID < $2
ORDER BY message.date ASC
`

const NewRecieptsQuery = `
SELECT chat.guid, message.guid, message.is_from_me, message.date_read
FROM message
JOIN chat_message_join ON chat_message_join.message_id = message.ROWID
JOIN chat              ON chat_message_join.chat_id = chat.ROWID
WHERE date_read>$1 AND is_read=1
`

const AttachmentsQuery = `
SELECT guid, COALESCE(filename, ''), COALESCE(mime_type, ''), transfer_name, is_sticker, sticker_user_info, COALESCE(emoji_image_short_description, '') FROM attachment
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

const GetChatIDsNames = `
tell application "Messages"
	set cs to get every chat
	set o to {}
	repeat with c in cs
		set i to get id of c
		set n to get name of c
		if n is missing value
			set n to ""
		end if
		set end of o to (i & "|" & n)
	end repeat
	set AppleScript's text item delimiters to "\n"
	copy o as string to stdout
end tell
`

const GetOwnContactIDs = `
tell application "Contacts"
	set o to {}
	set c to my card
	repeat with p in phones of c
		set end of o to value of p
	end repeat
	repeat with e in emails of c
		set end of o to value of e
	end repeat
	set AppleScript's text item delimiters to "
"
	copy o as string to stdout
end tell
`

const GetOwnContactFirstPhone = `
tell application "Contacts"	
	copy value of first phone of my card as string to stdout
end tell
`
