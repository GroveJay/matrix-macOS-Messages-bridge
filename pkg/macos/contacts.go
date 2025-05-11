package macos

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type ContactInformation struct {
	FirstName string
	LastName  string
	Nickname  string
	ID        string
}

type ContactsDB struct {
	db            *sql.DB
	dbPath        string
	contactsQuery *sql.Stmt
}

type MacOSContactsClient struct {
	contactsDBs []*ContactsDB
}

func createAndPrepareContactsDB(path string) (contactsDB *ContactsDB, err error) {
	contactsDB = &ContactsDB{
		dbPath: path,
	}
	if contactsDB.db, err = sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", path)); err != nil {
		return nil, err
	} else {
		if contactsDB.contactsQuery, err = contactsDB.db.Prepare(ContactsQuery); err != nil {
			return nil, err
		}
	}
	return contactsDB, nil
}

func openContactsDBs() (contactsDBs []*ContactsDB, err error) {
	path, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	contactsSourcesPath := filepath.Join(path, "Library", "Application Support", "AddressBook", "Sources")
	if sourcePaths, err := os.ReadDir(contactsSourcesPath); err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", contactsSourcesPath, err)
	} else {
		for _, sourcePath := range sourcePaths {
			if !sourcePath.IsDir() {
				continue
			}
			sourceSQLDBPath := filepath.Join(contactsSourcesPath, sourcePath.Name(), "AddressBook-v22.abcddb")
			if contactSourceDB, err := createAndPrepareContactsDB(sourceSQLDBPath); err != nil {
				return nil, fmt.Errorf("error opening contacts db %s: %w", sourceSQLDBPath, err)
			} else {
				contactsDBs = append(contactsDBs, contactSourceDB)
			}
		}
	}
	if len(contactsDBs) <= 0 {
		return nil, fmt.Errorf("did not find any Contacts databases")
	}
	return contactsDBs, nil
}

func GetContactsClient(userName string) (*MacOSContactsClient, error) {
	client := &MacOSContactsClient{}
	var err error
	if client.contactsDBs, err = openContactsDBs(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c MacOSContactsClient) ValidateConnection() error {
	return nil
}

func (c MacOSContactsClient) GetContactUserInfo(id string) (*bridgev2.UserInfo, error) {
	contactsMap, err := c.GetContactsMap()
	if err != nil {
		return nil, err
	}

	userInfo := &bridgev2.UserInfo{
		Identifiers: []string{},
	}
	if contactInformation, ok := contactsMap[networkid.UserID(id)]; ok {
		SupplementUserInfoWithContactInformation(userInfo, contactInformation, c)
	}
	return userInfo, nil
}

func (c MacOSContactsClient) GetContactsMap() (map[networkid.UserID]ContactInformation, error) {
	contactsMap := make(map[networkid.UserID]ContactInformation)
	errors := ""
	for _, contactsDB := range c.contactsDBs {
		fmt.Printf("Getting contacts from %s\n", contactsDB.dbPath)
		res, err := contactsDB.contactsQuery.Query()
		if err != nil {
			errors = errors + "\n" + err.Error()
			continue
		}
		for res.Next() {
			contactInformation := ContactInformation{}
			var phoneNumber string
			var email string
			err = res.Scan(&contactInformation.ID, &contactInformation.FirstName, &contactInformation.LastName, &contactInformation.Nickname, &phoneNumber, &email)
			if err != nil {
				errors = errors + "\n" + err.Error()
				continue
			}
			if len(email) != 0 {
				contactsMap[networkid.UserID(email)] = contactInformation
			}
			if len(phoneNumber) != 0 {
				if userID, err := ParseFormatPhoneNumber(phoneNumber, "US"); err != nil {
					errors = errors + "\n" + err.Error()
					continue
				} else {
					contactsMap[*userID] = contactInformation
				}
			}
		}
	}
	if len(contactsMap) == 0 {
		return nil, fmt.Errorf("didn't find any contacts (errors: %s)", errors)
	}
	fmt.Printf("Got errors getting contacts: %s\n", errors)
	return contactsMap, nil
}

func (c MacOSContactsClient) GetWrappedAvatarForID(ID string) *bridgev2.Avatar {
	if ID == "" {
		return &bridgev2.Avatar{Remove: true}
	}
	vcardResult, stderr, err := RunOsascript(GetContactVCard, ID)
	if err != nil || len(vcardResult) == 0 || len(stderr) != 0 {
		return &bridgev2.Avatar{Remove: true}
	}
	return &bridgev2.Avatar{
		ID: networkid.AvatarID(fmt.Sprintf("%s-avatar", ID)),
		Get: func(ctx context.Context) ([]byte, error) {
			return GetImageFromVCard(vcardResult)
		},
	}
}
