package gomine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"

	"encoding/hex"
	"errors"
	"fmt"
	"github.com/irmine/gomine/commands"
	"github.com/irmine/gomine/net"
	"github.com/irmine/gomine/net/info"
	"github.com/irmine/gomine/net/packets/data"
	"github.com/irmine/gomine/net/protocol"
	"github.com/irmine/gomine/packs"
	"github.com/irmine/gomine/permissions"
	"github.com/irmine/gomine/resources"
	"github.com/irmine/gomine/text"
	"github.com/irmine/gomine/worlds/generators"
	"github.com/irmine/goraklib/server"
	"github.com/irmine/query"
	"github.com/irmine/worlds"
	"github.com/irmine/worlds/providers"
	net2 "net"
	"os"
)

const (
	GoMineName    = "GoMine"
	GoMineVersion = "0.0.1"
)

type Server struct {
	isRunning         bool
	tick              int64
	privateKey        *ecdsa.PrivateKey
	token             []byte
	serverPath        string
	config            *resources.GoMineConfig
	consoleReader     *ConsoleReader
	commandHolder     *commands.Manager
	packManager       *packs.Manager
	permissionManager *permissions.Manager
	levelManager      *worlds.Manager
	sessionManager    *net.SessionManager
	networkAdapter    *net.NetworkAdapter
	pluginManager     *PluginManager
	queryManager      query.Manager
}

// AlreadyStarted gets returned during server startup,
// if the server has already been started.
var AlreadyStarted = errors.New("server is already started")

// NewServer returns a new server with the given server path.
func NewServer(serverPath string, config *resources.GoMineConfig) *Server {
	var s = &Server{}

	s.serverPath = serverPath
	s.config = config
	text.DefaultLogger.DebugMode = config.DebugMode
	file, _ := os.OpenFile("gomine.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0700)
	text.DefaultLogger.AddOutput(func(message []byte) {
		file.WriteString(text.ColoredString(message).StripAll())
	})

	s.levelManager = worlds.NewManager(serverPath)
	s.consoleReader = NewConsoleReader(s)
	s.commandHolder = commands.NewManager()

	s.sessionManager = net.NewSessionManager()
	s.networkAdapter = net.NewNetworkAdapter(s.sessionManager)
	s.networkAdapter.GetRakLibManager().PongData = s.GeneratePongData()
	s.networkAdapter.GetRakLibManager().RawPacketFunction = s.HandleRaw
	s.networkAdapter.GetRakLibManager().DisconnectFunction = s.HandleDisconnect

	s.RegisterDefaultProtocols()

	s.packManager = packs.NewManager(serverPath)

	s.permissionManager = permissions.NewManager()

	s.pluginManager = NewPluginManager(s)

	s.queryManager = query.NewManager()

	if config.UseEncryption {
		var curve = elliptic.P384()

		var err error
		s.privateKey, err = ecdsa.GenerateKey(curve, rand.Reader)
		text.DefaultLogger.LogError(err)

		if !curve.IsOnCurve(s.privateKey.X, s.privateKey.Y) {
			text.DefaultLogger.Error("Invalid private key generated")
		}

		var token = make([]byte, 128)
		rand.Read(token)
		s.token = token
	}

	return s
}

// RegisterDefaultProtocols registers all default protocols of GoMine.
func (server *Server) RegisterDefaultProtocols() {
	server.networkAdapter.GetProtocolManager().RegisterProtocol(NewP160(server))
	server.networkAdapter.GetProtocolManager().RegisterProtocol(NewP200(server))
	server.networkAdapter.GetProtocolManager().RegisterProtocol(NewP201(server))
	server.networkAdapter.GetProtocolManager().RegisterProtocol(NewP220(server))
}

// RegisterDefaultCommands registers all default commands of the server.
func (server *Server) RegisterDefaultCommands() {
	server.commandHolder.RegisterCommand(NewStop(server))
	server.commandHolder.RegisterCommand(NewList(server))
	server.commandHolder.RegisterCommand(NewPing())
	server.commandHolder.RegisterCommand(NewTest(server))
}

// IsRunning checks if the server is running.
func (server *Server) IsRunning() bool {
	return server.isRunning
}

// Start starts the server and loads levels, plugins, resource packs etc.
// Start returns an error if one occurred during starting.
func (server *Server) Start() error {
	if server.isRunning {
		return AlreadyStarted
	}
	text.DefaultLogger.Info("GoMine "+GoMineVersion+" is now starting...", "("+server.GetServerPath()+")")

	server.levelManager.SetDefaultLevel(worlds.NewLevel("world", server.GetServerPath()))
	var dimension = worlds.NewDimension("overworld", server.levelManager.GetDefaultLevel(), worlds.OverworldId)
	server.levelManager.GetDefaultLevel().SetDefaultDimension(dimension)
	dimension.SetChunkProvider(providers.NewAnvil(server.GetServerPath() + "worlds/world/overworld/region/"))
	dimension.SetGenerator(generators.Flat{})

	server.RegisterDefaultCommands()

	server.packManager.LoadResourcePacks() // Behavior packs may depend on resource packs, so always load resource packs first.
	server.packManager.LoadBehaviorPacks()

	server.pluginManager.LoadPlugins()

	server.isRunning = true
	return server.networkAdapter.GetRakLibManager().Start(server.config.ServerIp, int(server.config.ServerPort))
}

// Shutdown shuts down the server, saving and disabling everything.
func (server *Server) Shutdown() {
	if !server.isRunning {
		return
	}
	text.DefaultLogger.Info("Server is shutting down.")

	text.DefaultLogger.Notice("Server stopped.")
	text.DefaultLogger.Wait()

	server.isRunning = false
}

// GetMinecraftVersion returns the latest Minecraft game version.
// It is prefixed with a 'v', for example: "v1.2.10.1"
func (server *Server) GetMinecraftVersion() string {
	return info.LatestGameVersion
}

// GetMinecraftNetworkVersion returns the latest Minecraft network version.
// For example: "1.2.10.1"
func (server *Server) GetMinecraftNetworkVersion() string {
	return info.LatestGameVersionNetwork
}

// GetServerPath returns the server path the server is installed in.
func (server *Server) GetServerPath() string {
	return server.serverPath
}

// GetConfiguration returns the configuration file of GoMine.
func (server *Server) GetConfiguration() *resources.GoMineConfig {
	return server.config
}

// GetConsoleReader returns the console command reader.
func (server *Server) GetConsoleReader() *ConsoleReader {
	return server.consoleReader
}

// GetCommandHolder returns the command manager.
func (server *Server) GetCommandManager() *commands.Manager {
	return server.commandHolder
}

// HasPermission returns if the server has a given permission.
// Always returns true to satisfy the ICommandSender interface.
func (server *Server) HasPermission(string) bool {
	return true
}

// SendMessage sends a message to the server to satisfy the ICommandSender interface.
func (server *Server) SendMessage(message ...interface{}) {
	text.DefaultLogger.Notice(message)
}

// GetEngineName returns 'GoMine'.
func (server *Server) GetEngineName() string {
	return GoMineName
}

// GetName returns the LAN name of the server specified in the configuration.
func (server *Server) GetName() string {
	return server.config.ServerName
}

// GetPort returns the port of the server specified in the configuration.
func (server *Server) GetPort() uint16 {
	return server.config.ServerPort
}

// GetAddress returns the IP address specified in the configuration.
func (server *Server) GetAddress() string {
	return server.config.ServerIp
}

// GetMaximumPlayers returns the maximum amount of players on the server.
func (server *Server) GetMaximumPlayers() uint {
	return server.config.MaximumPlayers
}

// GetNetworkAdapter returns the NetworkAdapter of the server.
func (server *Server) GetNetworkAdapter() *net.NetworkAdapter {
	return server.networkAdapter
}

// Returns the Message Of The Day of the server.
func (server *Server) GetMotd() string {
	return server.config.ServerMotd
}

// GetPermissionManager returns the permission manager of the server.
func (server *Server) GetPermissionManager() *permissions.Manager {
	return server.permissionManager
}

// GetLevelManager returns the level manager of the server.
func (server *Server) GetLevelManager() *worlds.Manager {
	return server.levelManager
}

// GetSessionManager returns the Minecraft session manager of the server.
func (server *Server) GetSessionManager() *net.SessionManager {
	return server.sessionManager
}

// GetCurrentTick returns the current tick the server is on.
func (server *Server) GetCurrentTick() int64 {
	return server.tick
}

// GetPackManager returns the resource and behavior pack manager.
func (server *Server) GetPackManager() *packs.Manager {
	return server.packManager
}

// GetPluginManager returns the plugin manager of the server.
func (server *Server) GetPluginManager() *PluginManager {
	return server.pluginManager
}

// BroadcastMessageTo broadcasts a message to all receivers.
func (server *Server) BroadcastMessageTo(receivers []*net.MinecraftSession, message ...interface{}) {
	for _, session := range receivers {
		session.SendMessage(message)
	}
	text.DefaultLogger.LogChat(message)
}

// Broadcast broadcasts a message to all players and the console in the server.
func (server *Server) BroadcastMessage(message ...interface{}) {
	for _, session := range server.GetSessionManager().GetSessions() {
		session.SendMessage(message)
	}
	text.DefaultLogger.LogChat(message)
}

// GetPrivateKey returns the ECDSA private key of the server.
func (server *Server) GetPrivateKey() *ecdsa.PrivateKey {
	return server.privateKey
}

// GetPublicKey returns the ECDSA public key of the private key of the server.
func (server *Server) GetPublicKey() *ecdsa.PublicKey {
	return &server.privateKey.PublicKey
}

// GetServerToken returns the server token byte sequence.
func (server *Server) GetServerToken() []byte {
	return server.token
}

// GenerateQueryResult returns the query data of the server in a byte array.
func (server *Server) GenerateQueryResult() query.Result {
	var plugs []string
	for _, plug := range server.pluginManager.GetPlugins() {
		plugs = append(plugs, plug.GetName()+" v"+plug.GetVersion())
	}

	var ps []string
	for name := range server.sessionManager.GetSessions() {
		ps = append(ps, name)
	}

	var result = query.Result{
		MOTD:           server.GetMotd(),
		ListPlugins:    server.config.AllowPluginQuery,
		PluginNames:    plugs,
		PlayerNames:    ps,
		GameMode:       "SMP",
		Version:        server.GetMinecraftVersion(),
		ServerEngine:   server.GetEngineName(),
		WorldName:      server.levelManager.GetDefaultLevel().GetName(),
		OnlinePlayers:  int(server.GetSessionManager().GetSessionCount()),
		MaximumPlayers: int(server.config.MaximumPlayers),
		Whitelist:      "off",
		Port:           server.config.ServerPort,
		Address:        server.config.ServerIp,
	}

	return result
}

// HandleRaw handles a raw packet, for instance a query packet.
func (server *Server) HandleRaw(packet []byte, addr *net2.UDPAddr) {
	if string(packet[0:2]) == string(query.Header) {
		if !server.config.AllowQuery {
			return
		}

		var q = query.NewFromRaw(packet, addr)
		q.DecodeServer()

		server.queryManager.HandleQuery(q)
		return
	}
	text.DefaultLogger.Debug("Unhandled raw packet:", hex.EncodeToString(packet))
}

// HandleDisconnect handles a disconnection from a session.
func (server *Server) HandleDisconnect(s *server.Session) {
	text.DefaultLogger.Debug(s, "disconnected!")
	session, ok := server.GetSessionManager().GetSessionByRakNetSession(s)

	server.GetSessionManager().RemoveMinecraftSession(session)
	if !ok {
		return
	}

	if session.GetPlayer().Dimension != nil {
		for _, online := range server.GetSessionManager().GetSessions() {
			online.SendPlayerList(data.ListTypeRemove, map[string]protocol.PlayerListEntry{online.GetPlayer().GetName(): online.GetPlayer()})
		}

		session.GetPlayer().DespawnFromAll()

		session.GetPlayer().Close()

		server.BroadcastMessage(text.Yellow+session.GetDisplayName(), "has left the server")
	}
}

// GeneratePongData generates the GoRakLib pong data for the UnconnectedPong RakNet packet.
func (server *Server) GeneratePongData() string {
	return fmt.Sprint("MCPE;", server.GetMotd(), ";", info.LatestProtocol, ";", server.GetMinecraftNetworkVersion(), ";", server.GetSessionManager().GetSessionCount(), ";", server.config.MaximumPlayers, ";", server.networkAdapter.GetRakLibManager().ServerId, ";", server.GetEngineName(), ";Creative;")
}

// Tick ticks the entire server. (Levels, scheduler, GoRakLib server etc.)
// Internal. Not to be used by plugins.
func (server *Server) Tick() {
	if !server.isRunning {
		return
	}
	if server.tick%20 == 0 {
		server.queryManager.SetQueryResult(server.GenerateQueryResult())
		server.networkAdapter.GetRakLibManager().PongData = server.GeneratePongData()
	}

	for _, session := range server.sessionManager.GetSessions() {
		session.Tick()
	}

	for range server.levelManager.GetLevels() {
		//level.Tick()
	}
	server.tick++
}