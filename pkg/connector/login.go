package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/GroveJay/matrix-macOS-Messages-bridge/pkg/macos"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type MessagesLogin struct {
	User      *bridgev2.User
	Connector *MessagesConnector
	UserID    string
}

var _ bridgev2.LoginProcessUserInput = (*MessagesLogin)(nil)

func (m *MessagesConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{{
		Name:        "Confirm user phone id",
		Description: "Confirm the user phone id for messages and contacts",
		ID:          "user-id",
	}}
}

func (m *MessagesConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != "user-id" {
		return nil, fmt.Errorf("unknown login flow ID: %s", flowID)
	}
	return &MessagesLogin{
		User: user,
	}, nil
}

func (m *MessagesLogin) Cancel() {
	// TODO: any teardown?
}

func (m *MessagesLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	stdout, stderr, err := macos.RunOsascript(macos.GetOwnContactFirstPhone)
	if err != nil || len(stdout) == 0 || len(stderr) != 0 {
		return nil, fmt.Errorf("error getting user contact phone number: %w\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	// TODO: remove US assumption?
	maybePhone := strings.TrimSuffix(stdout, "\n")
	formattedPhoneNumber, err := macos.ParseFormatPhoneNumber(maybePhone, "US")
	if err != nil {
		return nil, fmt.Errorf("error parsing phone number (%s): %w", maybePhone, err)
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "messagesclient.enter_user_id",
		Instructions: fmt.Sprintf("Approve the user ID (phone number) associated with your user: %s", string(*formattedPhoneNumber)),
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:    bridgev2.LoginInputFieldTypePhoneNumber,
					ID:      "user_id",
					Name:    "User ID (phone number)",
					Pattern: "^+[0-9]+$",
				},
			},
		},
	}, nil
}

func (m *MessagesLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	m.UserID = input["user_id"]
	userLogin, err := m.User.NewLogin(ctx, &database.UserLogin{
		ID:         networkid.UserLoginID(m.UserID),
		RemoteName: m.UserID,
		Metadata: &UserLoginMetadata{
			UserID: m.UserID,
		},
	}, &bridgev2.NewLoginParams{
		LoadUserLogin: func(ctx context.Context, login *bridgev2.UserLogin) (err error) {
			login.Client = &MessagesClient{
				UserLogin: login,
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       "messagesclient.complete",
		Instructions: "Successfully input contact phone number",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLogin: userLogin,
		},
	}, nil
}
