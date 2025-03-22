
package board

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"
	log "github.com/spf13/jwalterweatherman"
	db "github.com/replit/database-go"
)

type PersistentBoardServer struct {
	activeGames sync.Map // map[string]*BoardServer
	httpServer  *http.Server
}

func NewPersistentBoardServer() *PersistentBoardServer {
	mux := http.NewServeMux()
	server := &PersistentBoardServer{
		httpServer: &http.Server{
			Handler: cors.New(cors.Options{
				AllowedOrigins:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
				AllowCredentials: true,
			}).Handler(mux),
		},
	}

	mux.HandleFunc("/games/", server.handleGameRequest)
	return server
}

func (server *PersistentBoardServer) handleGameRequest(w http.ResponseWriter, r *http.Request) {
	// Extract game ID from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}
	gameID := parts[2]

	// Handle websocket endpoint
	if len(parts) == 4 && parts[3] == "events" {
		server.handleWebsocket(gameID, w, r)
		return
	}

	// Handle game metadata endpoint
	server.handleGameMetadata(gameID, w, r)
}

func (server *PersistentBoardServer) handleGameMetadata(gameID string, w http.ResponseWriter, r *http.Request) {
	// Check for active game
	if activeServer, ok := server.activeGames.Load(gameID); ok {
		boardServer := activeServer.(*BoardServer)
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Game Game
		}{boardServer.game})
		return
	}

	// Load from database
	gameData, err := db.Get(gameID)
	if err != nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	var game Game
	if err := json.Unmarshal([]byte(gameData), &game); err != nil {
		http.Error(w, "Invalid game data", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Game Game
	}{game})
}

func (server *PersistentBoardServer) handleWebsocket(gameID string, w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ERROR.Printf("Unable to upgrade connection: %v", err)
		return
	}

	defer ws.Close()

	// Check for active game
	if activeServer, ok := server.activeGames.Load(gameID); ok {
		boardServer := activeServer.(*BoardServer)
		server.streamEvents(ws, boardServer.events)
		return
	}

	// Load from database and stream saved events
	gameData, err := db.Get(gameID)
	if err != nil {
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Game not found"))
		return
	}

	var events []GameEvent
	if err := json.Unmarshal([]byte(gameData), &events); err != nil {
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Invalid game data"))
		return
	}

	// Stream events from stored data
	for _, event := range events {
		jsonStr, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if err := ws.WriteMessage(websocket.TextMessage, jsonStr); err != nil {
			break
		}
	}
}

func (server *PersistentBoardServer) streamEvents(ws *websocket.Conn, events <-chan GameEvent) {
	for event := range events {
		jsonStr, err := json.Marshal(event)
		if err != nil {
			log.ERROR.Printf("Unable to serialize event: %v", err)
			continue
		}
		if err := ws.WriteMessage(websocket.TextMessage, jsonStr); err != nil {
			break
		}
	}
}

func (server *PersistentBoardServer) RegisterGame(game Game) {
	boardServer := NewBoardServer(game)
	server.activeGames.Store(game.ID, boardServer)
}

func (server *PersistentBoardServer) Listen() (string, error) {
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return "", err
	}
	go func() {
		err = server.httpServer.Serve(listener)
		if err != http.ErrServerClosed {
			log.ERROR.Printf("Error in board HTTP server: %v", err)
		}
	}()

	url := "http://" + listener.Addr().String()
	return url, nil
}

func (server *PersistentBoardServer) Shutdown() {
	server.activeGames.Range(func(key, value interface{}) bool {
		boardServer := value.(*BoardServer)
		boardServer.Shutdown()
		return true
	})

	if err := server.httpServer.Shutdown(context.Background()); err != nil {
		log.ERROR.Printf("Error shutting down HTTP server: %v", err)
	}
}
