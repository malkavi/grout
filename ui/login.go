package ui

import (
	"fmt"
	"grout/constants"
	"grout/utils"
	"os"
	"strconv"
	"strings"
	"time"

	"grout/romm"

	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/i18n"
)

type loginInput struct {
	ExistingHost romm.Host
}

type loginOutput struct {
	Host   romm.Host
	Config *utils.Config
}

type loginAttemptResult struct {
	BadHost        bool
	BadCredentials bool
}

type LoginScreen struct{}

func newLoginScreen() *LoginScreen {
	return &LoginScreen{}
}

func (s *LoginScreen) draw(input loginInput) (ScreenResult[loginOutput], error) {
	host := input.ExistingHost

	items := []gabagool.ItemWithOptions{
		{
			Item: gabagool.MenuItem{
				Text: i18n.GetString("login_protocol"),
			},
			Options: []gabagool.Option{
				{DisplayName: i18n.GetString("login_protocol_http"), Value: "http://"},
				{DisplayName: i18n.GetString("login_protocol_https"), Value: "https://"},
			},
			SelectedOption: func() int {
				if strings.Contains(host.RootURI, "https") {
					return 1
				}
				return 0
			}(),
		},
		{
			Item: gabagool.MenuItem{
				Text: i18n.GetString("login_hostname"),
			},
			Options: []gabagool.Option{
				{
					Type:           gabagool.OptionTypeKeyboard,
					DisplayName:    removeScheme(host.RootURI),
					KeyboardPrompt: removeScheme(host.RootURI),
					Value:          removeScheme(host.RootURI),
				},
			},
		},
		{
			Item: gabagool.MenuItem{
				Text: i18n.GetString("login_port"),
			},
			Options: []gabagool.Option{
				{
					Type: gabagool.OptionTypeKeyboard,
					KeyboardPrompt: func() string {
						if host.Port == 0 {
							return ""
						}
						return strconv.Itoa(host.Port)
					}(),
					DisplayName: func() string {
						if host.Port == 0 {
							return ""
						}
						return strconv.Itoa(host.Port)
					}(),
					Value: func() string {
						if host.Port == 0 {
							return ""
						}
						return strconv.Itoa(host.Port)
					}(),
				},
			},
		},
		{
			Item: gabagool.MenuItem{
				Text: i18n.GetString("login_username"),
			},
			Options: []gabagool.Option{
				{
					Type:           gabagool.OptionTypeKeyboard,
					DisplayName:    host.Username,
					KeyboardPrompt: host.Username,
					Value:          host.Username,
				},
			},
		},
		{
			Item: gabagool.MenuItem{
				Text: i18n.GetString("login_password"),
			},
			Options: []gabagool.Option{
				{
					Type:           gabagool.OptionTypeKeyboard,
					Masked:         true,
					DisplayName:    host.Password,
					KeyboardPrompt: host.Password,
					Value:          host.Password,
				},
			},
		},
	}

	res, err := gabagool.OptionsList(
		i18n.GetString("login_title"),
		gabagool.OptionListSettings{
			DisableBackButton: false,
			FooterHelpItems: []gabagool.FooterHelpItem{
				{ButtonName: "B", HelpText: i18n.GetString("button_quit")},
				{ButtonName: "←→", HelpText: i18n.GetString("button_cycle")},
				{ButtonName: "Start", HelpText: i18n.GetString("button_login")},
			},
		},
		items,
	)

	if err != nil {
		return withCode(loginOutput{}, gabagool.ExitCodeCancel), nil
	}

	loginSettings := res.Items

	newHost := romm.Host{
		RootURI: fmt.Sprintf("%s%s", loginSettings[0].Value(), loginSettings[1].Value()),
		Port: func(s string) int {
			if n, err := strconv.Atoi(s); err == nil {
				return n
			}
			return 0
		}(loginSettings[2].Value().(string)),
		Username: loginSettings[3].Options[0].Value.(string),
		Password: loginSettings[4].Options[0].Value.(string),
	}

	return success(loginOutput{Host: newHost}), nil
}

func LoginFlow(existingHost romm.Host) (*utils.Config, error) {
	screen := newLoginScreen()

	for {
		result, err := screen.draw(loginInput{ExistingHost: existingHost})
		if err != nil {
			gabagool.ProcessMessage(i18n.GetString("login_error_unexpected"), gabagool.ProcessMessageOptions{}, func() (interface{}, error) {
				time.Sleep(3 * time.Second)
				return nil, nil
			})
			return nil, fmt.Errorf("unable to get login information: %w", err)
		}

		if result.ExitCode == gabagool.ExitCodeBack || result.ExitCode == gabagool.ExitCodeCancel {
			os.Exit(1)
		}

		host := result.Value.Host

		loginResult := attemptLogin(host)

		switch {
		case loginResult.BadHost:
			gabagool.ConfirmationMessage(i18n.GetString("login_error_connection"),
				[]gabagool.FooterHelpItem{
					{ButtonName: "A", HelpText: i18n.GetString("button_continue")},
				},
				gabagool.MessageOptions{})
			existingHost = host
			continue

		case loginResult.BadCredentials:
			gabagool.ConfirmationMessage(i18n.GetString("login_error_credentials"),
				[]gabagool.FooterHelpItem{
					{ButtonName: "A", HelpText: i18n.GetString("button_continue")},
				},
				gabagool.MessageOptions{})
			existingHost = host
			continue
		}

		config := &utils.Config{
			Hosts: []romm.Host{host},
		}
		return config, nil
	}
}

func attemptLogin(host romm.Host) loginAttemptResult {
	rc := utils.GetRommClient(host, constants.LoginTimeout)

	result, _ := gabagool.ProcessMessage(i18n.GetString("login_logging_in"), gabagool.ProcessMessageOptions{}, func() (interface{}, error) {
		lr := rc.Login(host.Username, host.Password)
		if lr != nil {
			// Check if the error is a connection error (host unreachable)
			// Connection errors contain "failed to login:" or "failed to execute request"
			// Auth errors contain "login failed with status"
			errorMsg := lr.Error()
			if strings.Contains(errorMsg, "failed to login:") ||
				strings.Contains(errorMsg, "failed to execute request") ||
				strings.Contains(errorMsg, "failed to create") {
				return loginAttemptResult{BadHost: true}, nil
			}
			return loginAttemptResult{BadCredentials: true}, nil
		}
		return loginAttemptResult{}, nil
	})

	return result.(loginAttemptResult)
}

func removeScheme(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") {
		return strings.TrimPrefix(rawURL, "https://")
	}
	if strings.HasPrefix(rawURL, "http://") {
		return strings.TrimPrefix(rawURL, "http://")
	}
	return rawURL
}
