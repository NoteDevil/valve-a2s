package a2s

type ServerInfo struct {
	Protocol    byte
	Name        string
	Map         string
	Folder      string
	Game        string
	AppID       uint16
	Players     byte
	MaxPlayers  byte
	Bots        byte
	ServerType  byte
	Environment byte
	Visibility  byte
	VAC         byte
	Version     string
	GamePort    uint16
	SteamID     uint64
	SourceTV    struct {
		Port uint16
		Name string
	}
	Keywords []string
	GameID   uint64
	EDF      byte
}

type PlayerInfo struct {
	Index    byte
	Name     string
	Score    int32
	Duration float32
	Deaths   int32
	Money    int32
}

type Rule struct {
	Name  string
	Value string
}

type ServerFeatures struct {
	Info    bool
	Players bool
	Rules   bool
	Ping    bool
}