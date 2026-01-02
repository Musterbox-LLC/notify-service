package sse

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
)

// Event represents an SSE event
type Event struct {
	Type   string      `json:"type"`
	Data   interface{} `json:"data"`
	UserID uuid.UUID   `json:"user_id"`
}

// Broker manages SSE connections
type Broker struct {
	clients map[uuid.UUID]map[chan Event]bool
	mu      sync.RWMutex
}

// NewBroker creates a new SSE broker
func NewBroker() *Broker {
	return &Broker{
		clients: make(map[uuid.UUID]map[chan Event]bool),
	}
}

// Register adds a new client channel for a user
func (b *Broker) Register(userID uuid.UUID, clientChan chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if _, ok := b.clients[userID]; !ok {
		b.clients[userID] = make(map[chan Event]bool)
	}
	
	b.clients[userID][clientChan] = true
	log.Printf("📡 [SSE Broker] Registered client for user %s (total clients: %d)", 
		userID, len(b.clients[userID]))
}

// Unregister removes a client channel for a user
func (b *Broker) Unregister(userID uuid.UUID, clientChan chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if userClients, ok := b.clients[userID]; ok {
		delete(userClients, clientChan)
		close(clientChan)
		
		if len(userClients) == 0 {
			delete(b.clients, userID)
		}
		
		log.Printf("📡 [SSE Broker] Unregistered client for user %s (remaining: %d)", 
			userID, len(userClients))
	}
}

// Broadcast sends an event to all clients for a specific user
func (b *Broker) Broadcast(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if userClients, ok := b.clients[event.UserID]; ok {
		// Marshal data once for efficiency
		dataJSON, err := json.Marshal(event.Data)
		if err != nil {
			log.Printf("❌ [SSE Broker] Failed to marshal event data: %v", err)
			return
		}
		
		// Create event copy with marshaled data to avoid race conditions
		eventCopy := Event{
			Type:   event.Type,
			Data:   json.RawMessage(dataJSON),
			UserID: event.UserID,
		}
		
		for clientChan := range userClients {
			select {
			case clientChan <- eventCopy:
				// Successfully sent
			default:
				// Channel is full or blocked, skip this client
				log.Printf("⚠️ [SSE Broker] Client channel blocked for user %s", event.UserID)
			}
		}
		
		log.Printf("📡 [SSE Broker] Broadcast event %s to %d clients for user %s", 
			event.Type, len(userClients), event.UserID)
	} else {
		log.Printf("📡 [SSE Broker] No clients to broadcast to for user %s", event.UserID)
	}
}

// BroadcastToAll sends an event to all connected clients
func (b *Broker) BroadcastToAll(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	totalClients := 0
	for userID, userClients := range b.clients {
		totalClients += len(userClients)
		
		// Marshal data once for efficiency
		dataJSON, err := json.Marshal(event.Data)
		if err != nil {
			log.Printf("❌ [SSE Broker] Failed to marshal event data: %v", err)
			continue
		}
		
		// Create event copy with marshaled data
		eventCopy := Event{
			Type:   event.Type,
			Data:   json.RawMessage(dataJSON),
			UserID: userID,
		}
		
		for clientChan := range userClients {
			select {
			case clientChan <- eventCopy:
				// Successfully sent
			default:
				// Channel is full or blocked, skip this client
				log.Printf("⚠️ [SSE Broker] Client channel blocked for user %s", userID)
			}
		}
	}
	
	if totalClients > 0 {
		log.Printf("📡 [SSE Broker] Broadcast event %s to %d total clients", 
			event.Type, totalClients)
	}
}

// GetClientCount returns the number of connected clients for a user
func (b *Broker) GetClientCount(userID uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	if userClients, ok := b.clients[userID]; ok {
		return len(userClients)
	}
	return 0
}

// GetTotalClientCount returns the total number of connected clients
func (b *Broker) GetTotalClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	total := 0
	for _, userClients := range b.clients {
		total += len(userClients)
	}
	return total
}