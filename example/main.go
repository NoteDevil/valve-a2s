package main

import (
	"fmt"
	"log"
	"time"

	a2s "github.com/notedevil/valve-a2s"
)

// main demonstrates the usage of the library by connecting to a server and
// retrieving its details, players, and rules.
func main() {
	client := a2s.NewClient(5 * time.Second)
	defer client.Close()

	serverAddr := "45.62.160.32:27015"
	if err := client.Connect(serverAddr); err != nil {
		log.Fatalf("Connection error: %v", err)
	}

	info, err := client.GetInfo()
	if err != nil {
		log.Fatalf("Error getting info: %v", err)
	}

	fmt.Printf("Server: %s\n", info.Name)
	fmt.Printf("Map: %s\n", info.Map)
	fmt.Printf("Players: %d/%d\n", info.Players, info.MaxPlayers)
	fmt.Printf("Version: %s\n", info.Version)


	features := client.CheckFeatures()
	
	if features.Players {
		players, err := client.GetPlayers()
		if err != nil {
			log.Printf("Error getting players: %v", err)
		} else {
			fmt.Printf("\nPlayers online: %d\n", len(players))
			for _, player := range players {
				fmt.Printf("  %s (Score: %d)\n", player.Name, player.Score)
			}
		}
	}

	if features.Rules {
		rules, err := client.GetRules()
		if err != nil {
			log.Printf("Error getting rules: %v", err)
		} else {
			fmt.Printf("\nServer rules: %d\n", len(rules))
			for i, rule := range rules {
				if i >= 5 {
					fmt.Printf("  ... and %d more\n", len(rules)-5)
					break
				}
				fmt.Printf("  %s = %s\n", rule.Name, rule.Value)
			}
		}
	}
}