package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"malten.ai/spatial"
)

// AgentsHandler handles /agents endpoint
// GET /agents - list all agents
// GET /agents/{id} or /agents?id=xxx - get specific agent
// POST /agents - create agent (lat, lon, prompt params)
// POST /agents/{id} - instruct agent (action, prompt, lat, lon params)
// DELETE /agents/{id} - kill agent
//
// Accepts form params, query params, or JSON body (if Content-Type: application/json)
func AgentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Parse form/query params
	r.ParseForm()
	
	// Get agent ID from path or query
	path := strings.TrimPrefix(r.URL.Path, "/agents")
	agentID := strings.TrimPrefix(path, "/")
	if agentID == "" {
		agentID = r.Form.Get("id")
	}
	
	switch r.Method {
	case "GET":
		if agentID == "" {
			listAgents(w, r)
		} else {
			getAgent(w, r, agentID)
		}
	case "POST":
		if agentID == "" {
			createAgent(w, r)
		} else {
			instructAgent(w, r, agentID)
		}
	case "DELETE":
		if agentID == "" {
			JsonError(w, "agent id required", 400)
		} else {
			deleteAgent(w, r, agentID)
		}
	default:
		JsonError(w, "method not allowed", 405)
	}
}

// GET /agents - list all agents
func listAgents(w http.ResponseWriter, r *http.Request) {
	db := spatial.Get()
	agents := db.ListAgents()
	
	var result []map[string]interface{}
	for _, a := range agents {
		result = append(result, agentToJSON(a))
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": result,
		"count":  len(result),
	})
}

// GET /agents/{id} - get specific agent
func getAgent(w http.ResponseWriter, r *http.Request, id string) {
	db := spatial.Get()
	agent := db.GetByID(id)
	
	if agent == nil || agent.Type != spatial.EntityAgent {
		JsonError(w, "agent not found", 404)
		return
	}
	
	json.NewEncoder(w).Encode(agentToJSON(agent))
}

// POST /agents - create agent at location
// Params: lat, lon, prompt
func createAgent(w http.ResponseWriter, r *http.Request) {
	lat, _ := strconv.ParseFloat(r.Form.Get("lat"), 64)
	lon, _ := strconv.ParseFloat(r.Form.Get("lon"), 64)
	prompt := r.Form.Get("prompt")
	
	// Also try JSON body if content-type is set
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
			Prompt string  `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.Lat != 0 {
				lat = req.Lat
			}
			if req.Lon != 0 {
				lon = req.Lon
			}
			if req.Prompt != "" {
				prompt = req.Prompt
			}
		}
	}
	
	if lat == 0 && lon == 0 {
		JsonError(w, "lat and lon required", 400)
		return
	}
	
	db := spatial.Get()
	agent := db.FindOrCreateAgent(lat, lon)
	
	// Note: prompt not stored in typed AgentEntityData - stored elsewhere if needed
	_ = prompt
	
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(agentToJSON(agent))
}

// POST /agents/{id} - send instruction to agent
// Params: action (refresh|move|pause|resume), prompt, lat, lon
func instructAgent(w http.ResponseWriter, r *http.Request, id string) {
	db := spatial.Get()
	agent := db.GetByID(id)
	
	if agent == nil || agent.Type != spatial.EntityAgent {
		JsonError(w, "agent not found", 404)
		return
	}
	
	action := r.Form.Get("action")
	prompt := r.Form.Get("prompt")
	lat, _ := strconv.ParseFloat(r.Form.Get("lat"), 64)
	lon, _ := strconv.ParseFloat(r.Form.Get("lon"), 64)
	
	// Also try JSON body if content-type is set
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			Action string  `json:"action"`
			Prompt string  `json:"prompt"`
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.Action != "" {
				action = req.Action
			}
			if req.Prompt != "" {
				prompt = req.Prompt
			}
			if req.Lat != 0 {
				lat = req.Lat
			}
			if req.Lon != 0 {
				lon = req.Lon
			}
		}
	}
	
	agentData := agent.GetAgentData()
	if agentData == nil {
		agentData = &spatial.AgentEntityData{}
	}
	
	switch action {
	case "refresh":
		agentData.Status = "refreshing"
	case "move":
		if lat != 0 || lon != 0 {
			agent.Lat = lat
			agent.Lon = lon
		}
	case "pause":
		agentData.Status = "paused"
	case "resume":
		agentData.Status = "active"
	}
	
	// Note: prompt not stored in AgentEntityData
	_ = prompt
	
	agent.Data = agentData
	db.Insert(agent)
	json.NewEncoder(w).Encode(agentToJSON(agent))
}

// DELETE /agents/{id} - kill agent
func deleteAgent(w http.ResponseWriter, r *http.Request, id string) {
	db := spatial.Get()
	agent := db.GetByID(id)
	
	if agent == nil || agent.Type != spatial.EntityAgent {
		JsonError(w, "agent not found", 404)
		return
	}
	
	db.Delete(id)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted": id,
		"ok":      true,
	})
}

func agentToJSON(a *spatial.Entity) map[string]interface{} {
	result := map[string]interface{}{
		"id":        a.ID,
		"name":      a.Name,
		"lat":       a.Lat,
		"lon":       a.Lon,
		"updatedAt": a.UpdatedAt,
	}
	
	if agentData := a.GetAgentData(); agentData != nil {
		result["status"] = agentData.Status
		result["radius"] = agentData.Radius
		result["poiCount"] = agentData.POICount
		if agentData.LastIndex != nil {
			result["lastIndex"] = agentData.LastIndex.Format("2006-01-02T15:04:05Z")
		}
	}
	
	return result
}
