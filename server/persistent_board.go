
package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/BattlesnakeOfficial/rules/board"
	db "github.com/replit/database-go"
)

type PersistentBoardServer struct {
	activeGames map[string]*GameState
	eventChans  map[string][]chan board.GameEvent
	mu          sync.RWMutex
}

type GameState struct {
	game    board.Game
	events  []board.GameEvent
	isLive  bool
}

func NewPersistentBoardServer() *PersistentBoardServer {
	return &PersistentBoardServer{
		activeGames: make(map[string]*GameState),
		eventChans:  make(map[string][]chan board.GameEvent),
	}
}

func (s *PersistentBoardServer) AddGame(game board.Game) *GameState {
	s.mu.Lock()
	defer s.mu.Unlock()

	gameState := &GameState{
		game:    game,
		events:  make([]board.GameEvent, 0),
		isLive:  true,
	}
	s.activeGames[game.ID] = gameState
	s.eventChans[game.ID] = make([]chan board.GameEvent, 0)
	return gameState
}

func (s *PersistentBoardServer) SendEvent(gameID string, event board.GameEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if gameState, exists := s.activeGames[gameID]; exists {
		gameState.events = append(gameState.events, event)
		
		// Send to all active websocket connections
		for _, ch := range s.eventChans[gameID] {
			ch <- event
		}

		// If this is a game end event, persist to DB and cleanup
		if event.EventType == board.EVENT_TYPE_GAME_END {
			gameState.isLive = false
			eventsJSON, _ := json.Marshal(gameState.events)
			db.Set(fmt.Sprintf("game:%s:events", gameID), string(eventsJSON))
			gameJSON, _ := json.Marshal(gameState.game)
			db.Set(fmt.Sprintf("game:%s:metadata", gameID), string(gameJSON))
		}
	}
}

func (s *PersistentBoardServer) SubscribeToGame(gameID string) (chan board.GameEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// First check active games
	if gameState, exists := s.activeGames[gameID]; exists {
		ch := make(chan board.GameEvent, 100)
		s.eventChans[gameID] = append(s.eventChans[gameID], ch)
		
		// Send existing events
		go func() {
			for _, event := range gameState.events {
				ch <- event
			}
		}()
		
		return ch, nil
	}

	// Check DB for completed games
	eventsJSON, err := db.Get(fmt.Sprintf("game:%s:events", gameID))
	if err != nil {
		return nil, fmt.Errorf("game not found: %s", gameID)
	}

	ch := make(chan board.GameEvent, 100)
	var events []board.GameEvent
	json.Unmarshal([]byte(eventsJSON), &events)

	// Send stored events
	go func() {
		for _, event := range events {
			ch <- event
		}
		close(ch)
	}()

	return ch, nil
}

func (s *PersistentBoardServer) GetGame(gameID string) (*board.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if gameState, exists := s.activeGames[gameID]; exists {
		return &gameState.game, nil
	}

	// Check DB for completed games
	gameJSON, err := db.Get(fmt.Sprintf("game:%s:metadata", gameID))
	if err != nil {
		return nil, fmt.Errorf("game not found: %s", gameID)
	}

	var game board.Game
	json.Unmarshal([]byte(gameJSON), &game)
	return &game, nil
}
