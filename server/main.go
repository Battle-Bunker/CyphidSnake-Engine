package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/BattlesnakeOfficial/rules/cli/commands"
	db "github.com/replit/database-go"
)

// Amend PlayRequest to accept a collection of players
type PlayRequest struct {
	Players []commands.Player `json:"players"`
	// Players []string `json:"players"`
}

// Define a struct to hold the JSON data for index response
type IndexResponse struct {
	Status string `json:"status"`
}

var persistentServer *board.PersistentBoardServer

func main() {
	// Create persistent board server
	persistentServer = board.NewPersistentBoardServer()
	serverURL, err := persistentServer.Listen()
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to start persistent server: %v", err))
	}
	defer persistentServer.Shutdown()

	// Set up routes
	http.HandleFunc("/play", playHandler)
	http.HandleFunc("/", indexHandler)

	fmt.Printf("Board server listening on %s\n", serverURL)
	fmt.Println("Game server is running on http://0.0.0.0:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// playHandler handles the POST request to the '/play' endpoint
func playHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PlayRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create a temporary file for the game output
	tmpFile, err := os.CreateTemp("", "game_output_*.json")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name()) // Clean up the file afterwards

	// Call PlayBattlesnakeGame with the collection of players and output path
	gameID, err := commands.PlayBattlesnakeGame(req.Players, tmpFile.Name())
	if err != nil {
		http.Error(w, fmt.Sprintf("Error playing game: %v", err), http.StatusInternalServerError)
		return
	}
	// Store in key value database
	// Read the contents of the temporary file
	gameData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading temp file: %v", err), http.StatusInternalServerError)
		return
	}

	fmt.Printf("Game %v started\n", gameID)
	// fmt.Println("Game Data: %v", string(gameData))

	// Store the game data in the Replit database
	err = db.Set(gameID, string(gameData))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error storing game data: %v", err), http.StatusInternalServerError)
		return
	}

	// Send the game file as a response
	// http.ServeFile(w, r, tmpFile.Name())

	// Create and register the game with persistent server
	boardGame := board.Game{
		ID:     gameID,
		Status: "running",
		Width:  11,  // You may want to make these configurable
		Height: 11,
		Ruleset: map[string]string{
			"name": "standard",
		},
		RulesetName: "standard",
		RulesStages: []string{},
	}
	persistentServer.RegisterGame(boardGame)

	// Store the game data for later retrieval
	err = db.Set(gameID, string(gameData))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error storing game data: %v", err), http.StatusInternalServerError)
		return
	}

	boardURL := os.Getenv("BOARD_URL")
	if boardURL == "" {
		boardURL = "https://board.battlesnake.com"
	}
	gameURL := fmt.Sprintf("%s/?game=%s", boardURL, gameID)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(gameURL))

	//TODO: return url to this game running on Board e.g. %BOARD_URL%?game=$gameID
	// return fmt.Sprintf("%s/?game=%s", boardURL, gameID)
}

// indexHandler handles the GET requests to the index page
func indexHandler(w http.ResponseWriter, r *http.Request) {
	// Check if the request method is GET
	if r.Method != "GET" {
		http.Error(w, "Only GET method is allowed", http.StatusMethodNotAllowed)
		return
	}
	// Create response object
	response := IndexResponse{Status: "Ok"}
	// Set Content-Type header
	w.Header().Set("Content-Type", "application/json")
	// Encode the response as JSON and send it
	json.NewEncoder(w).Encode(response)
}
