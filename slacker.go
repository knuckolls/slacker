package slacker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nlopes/slack"
	"github.com/shomali11/proper"
)

const (
	space               = " "
	dash                = "-"
	newLine             = "\n"
	invalidToken        = "invalid token"
	helpCommand         = "help"
	directChannelMarker = "D"
	userMentionFormat   = "<@%s>"
	codeMessageFormat   = "`%s`"
	boldMessageFormat   = "*%s*"
	italicMessageFormat = "_%s_"
	slackBotUser        = "USLACKBOT"
)

// NewClient creates a new client using the Slack API
func NewClient(token string) *Slacker {
	client := slack.New(token)
	slacker := &Slacker{
		Client: client,
		RTM:    client.NewRTM(),
	}
	return slacker
}

// Slacker contains the Slack API, botCommands, and handlers
type Slacker struct {
	Client                *slack.Client
	RTM                   *slack.RTM
	botCommands           []*BotCommand
	initHandler           func()
	errorHandler          func(err string)
	helpHandler           func(request *Request, response ResponseWriter)
	defaultMessageHandler func(request *Request, response ResponseWriter)
	defaultEventHandler   func(interface{})
}

// Init handle the event when the bot is first connected
func (s *Slacker) Init(initHandler func()) {
	s.initHandler = initHandler
}

// Err handle when errors are encountered
func (s *Slacker) Err(errorHandler func(err string)) {
	s.errorHandler = errorHandler
}

// DefaultCommand handle messages when none of the commands are matched
func (s *Slacker) DefaultCommand(defaultMessageHandler func(request *Request, response ResponseWriter)) {
	s.defaultMessageHandler = defaultMessageHandler
}

// DefaultEvent handle events when an unknown event is seen
func (s *Slacker) DefaultEvent(defaultEventHandler func(interface{})) {
	s.defaultEventHandler = defaultEventHandler
}

// Help handle the help message, it will use the default if not set
func (s *Slacker) Help(helpHandler func(request *Request, response ResponseWriter)) {
	s.helpHandler = helpHandler
}

// Command define a new command and append it to the list of existing commands
func (s *Slacker) Command(usage string, description string, handler func(request *Request, response ResponseWriter)) {
	s.botCommands = append(s.botCommands, NewBotCommand(usage, description, handler))
}

// Listen receives events from Slack and each is handled as needed
func (s *Slacker) Listen() error {
	s.prependHelpHandle()

	go s.RTM.ManageConnection()

	for msg := range s.RTM.IncomingEvents {
		switch event := msg.Data.(type) {
		case *slack.ConnectedEvent:
			if s.initHandler == nil {
				continue
			}
			go s.initHandler()

		case *slack.MessageEvent:
			/*if s.isFromBot(event) {
				fmt.Printf("dropping from bot: %#v\n", event)
				continue
			}*/

			if !s.isBotMentioned(event) && !s.isDirectMessage(event) {
				fmt.Printf("dropping not mentioned or not direct message: %#v\n", event)
				continue
			}
			fmt.Printf("handling message: %#v\n", event)
			go s.handleMessage(event)

		case *slack.RTMError:
			if s.errorHandler == nil {
				continue
			}
			go s.errorHandler(event.Error())

		case *slack.InvalidAuthEvent:
			return errors.New(invalidToken)

		default:
			if s.defaultEventHandler == nil {
				continue
			}
			go s.defaultEventHandler(event)
		}
	}
	return nil
}

func (s *Slacker) sendMessage(text string, channel string) {
	s.RTM.SendMessage(s.RTM.NewOutgoingMessage(text, channel))
}

func (s *Slacker) isFromBot(event *slack.MessageEvent) bool {
	info := s.RTM.GetInfo()
	return len(event.User) == 0 || event.User == slackBotUser || event.User == info.User.ID || len(event.BotID) > 0
}

func (s *Slacker) isBotMentioned(event *slack.MessageEvent) bool {
	info := s.RTM.GetInfo()
	return strings.Contains(event.Text, fmt.Sprintf(userMentionFormat, info.User.ID)) || strings.Contains(event.Attachments[0].Pretext, fmt.Sprintf(userMentionFormat, info.User.ID))
}

func (s *Slacker) isDirectMessage(event *slack.MessageEvent) bool {
	return strings.HasPrefix(event.Channel, directChannelMarker)
}

func (s *Slacker) handleMessage(event *slack.MessageEvent) {
	response := NewResponse(event.Channel, s.RTM)
	ctx := context.Background()

	for _, cmd := range s.botCommands {
		textParameters, isTextMatch := cmd.Match(event.Text)
		attachmentParameters, isAttachmentMatch := cmd.Match(event.Attachments[0].Pretext)
		if isTextMatch {
			cmd.Execute(NewRequest(ctx, event, textParameters), response)
		} else if isAttachmentMatch {
			cmd.Execute(NewRequest(ctx, event, attachmentParameters), response)
		} else {
			continue
		}

		return

	}

	if s.defaultMessageHandler != nil {
		s.defaultMessageHandler(NewRequest(ctx, event, &proper.Properties{}), response)
	}
}

func (s *Slacker) defaultHelp(request *Request, response ResponseWriter) {
	helpMessage := empty
	for _, command := range s.botCommands {
		tokens := command.Tokenize()
		for _, token := range tokens {
			if token.IsParameter {
				helpMessage += fmt.Sprintf(codeMessageFormat, token.Word) + space
			} else {
				helpMessage += fmt.Sprintf(boldMessageFormat, token.Word) + space
			}
		}
		helpMessage += dash + space + fmt.Sprintf(italicMessageFormat, command.description) + newLine
	}
	response.Reply(helpMessage)
}

func (s *Slacker) prependHelpHandle() {
	if s.helpHandler == nil {
		s.helpHandler = s.defaultHelp
	}
	s.botCommands = append([]*BotCommand{NewBotCommand(helpCommand, helpCommand, s.helpHandler)}, s.botCommands...)
}
