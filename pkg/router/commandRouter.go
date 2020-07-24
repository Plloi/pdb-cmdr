package router

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/sdomino/scribble"
	log "github.com/sirupsen/logrus"
)

type MessageHandler func(*discordgo.Session, *discordgo.MessageCreate)

//CommandRouter Handles routing of chat commands to handler functions
type CommandRouter struct {
	commands      map[string]MessageHandler
	helpText      map[string]string
	DefaultPrefix string
	Servers       map[string]ServerSettings
	db            *scribble.Driver
}

type ServerSettings struct {
	Prefix  string
	GuildID string
}

// NewCommandRouter Sets up a new command router.
func NewCommandRouter() *CommandRouter {
	return NewCommandRouterWithPrefix("!")
}

func NewCommandRouterWithPrefix(prefix string) *CommandRouter {
	dir := "./settings"

	db, err := scribble.New(dir, nil)
	if err != nil {
		fmt.Println("Error", err)
	}

	//Load Pricedb
	servers, err := db.ReadAll("Servers")
	if err != nil {
		fmt.Println("Error", err)
	}

	router := &CommandRouter{
		commands: make(map[string]MessageHandler),
		helpText: make(map[string]string),
		Servers:  make(map[string]ServerSettings),
		db:       db,
	}

	//AddServerPrefixes to the Router
	for _, prefix := range servers {
		var serverSettings ServerSettings
		if err := json.Unmarshal(prefix, &serverSettings); err != nil {
			fmt.Println("Error", err)
		}
		router.Servers[serverSettings.GuildID] = serverSettings
	}

	router.DefaultPrefix = "!"
	router.RegisterCommand("help", "This help text", router.help)

	return router
}

func (c *CommandRouter) help(s *discordgo.Session, m *discordgo.MessageCreate) {
	var helpMessage = "Here's a list of available commands:\n"
	for key, value := range c.helpText {
		helpMessage += fmt.Sprintf("* %s: %s\n", key, value)
	}
	s.ChannelMessageSend(m.ChannelID, helpMessage)
}

// SetPrefix Set the bot's trigger prefix to message string, not included by default in the command list, make sure to register
func (c *CommandRouter) SetPrefix(s *discordgo.Session, m *discordgo.MessageCreate) {
	if ok, _ := MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator); ok {
		curPrefix := c.DefaultPrefix
		var server ServerSettings
		if _, ok := c.Servers[m.GuildID]; ok {
			server = c.Servers[m.GuildID]
			curPrefix = c.Servers[m.GuildID].Prefix
		} else {
			server = ServerSettings{
				Prefix:  c.DefaultPrefix,
				GuildID: m.GuildID,
			}
		}

		server.Prefix = m.Content
		if err := c.db.Write("Servers", server.GuildID, server); err != nil {
			fmt.Println("Error", err)
		}

		c.Servers[server.GuildID] = server

		if curPrefix != server.Prefix {
			s.ChannelMessageSend(m.ChannelID, "Prefix Updated")
		}

	}
}

// RegisterCommand Adds a command, it's help text, and function to the router. the help command is reserved
func (c *CommandRouter) RegisterCommand(command string, help string, f MessageHandler) error {
	if _, ok := c.commands[command]; ok {
		return fmt.Errorf("Command %s is already registered", command)
	}
	if _, ok := c.helpText[command]; ok {
		return fmt.Errorf("Help for command %s is already registered", command)
	}

	c.commands[command] = f
	c.helpText[command] = help
	return nil
}

// HandleCommand Takes Discord input and tries to find a relevant command, can be passed to discord-go's AddHandler
func (c *CommandRouter) HandleCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot
	if m.Author.ID == s.State.User.ID {
		return
	}

	prefixToCheck := c.DefaultPrefix
	if _, ok := c.Servers[m.GuildID]; ok && len(prefixToCheck) > 0 {
		prefixToCheck = c.Servers[m.GuildID].Prefix
	}

	// Check for bot trigger
	if len(m.Content) >= len(prefixToCheck) && m.Content[:len(prefixToCheck)] == prefixToCheck {
		// trim prefix
		m.Content = m.Content[len(prefixToCheck):]
		args := strings.Split(m.Content, " ")

		// Check if command is registered
		if f, ok := c.commands[args[0]]; ok {
			// Remove command from content, trim spaces
			m.Content = strings.TrimSpace(m.Content[len(args[0]):])
			// Call function
			log.Debugf("Calling handler for command: %s", args[0])
			f(s, m)
			return
		} else if len(args[0]) == 0 && len(prefixToCheck) == 0 {
			s.ChannelMessageSend(m.ChannelID, "Sub command needed. ")
		} else if len(args[0]) > 0 {
			s.ChannelMessageSend(m.ChannelID, "Command not recognized")
		}
		c.help(s, m)
	}
}

// MemberHasPermission checks if a member has the given permission
// for example, If you would like to check if user has the administrator
// permission you would use
// --- MemberHasPermission(s, guildID, userID, discordgo.PermissionAdministrator)
// If you want to check for multiple permissions you would use the bitwise OR
// operator to pack more bits in. (e.g): PermissionAdministrator|PermissionAddReactions
// =================================================================================
//     s          :  discordgo session
//     guildID    :  guildID of the member you wish to check the roles of
//     userID     :  userID of the member you wish to retrieve
//     permission :  the permission you wish to check for
func MemberHasPermission(s *discordgo.Session, guildID string, userID string, permission int) (bool, error) {
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		if member, err = s.GuildMember(guildID, userID); err != nil {
			return false, err
		}
	}

	// Iterate through the role IDs stored in member.Roles
	// to check permissions
	for _, roleID := range member.Roles {
		role, err := s.State.Role(guildID, roleID)
		if err != nil {
			return false, err
		}
		if role.Permissions&permission != 0 {
			return true, nil
		}
	}

	return false, nil
}
