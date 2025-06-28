package main

import (
	"net/http"
	"time"
	"unsafe"
	"fmt"
	"encoding/json"
	"os/exec"
	"bufio"
    "bytes"
	"github.com/fstanis/screenresolution"
	"syscall" 
	"github.com/asticode/go-astilectron"
	"github.com/shashankraocoding/go-shankskit"
	"golang.org/x/sys/windows"
)

var routes = map[string]http.HandlerFunc{
	"/": chat,
	"/message" : message, 
}
var resolution = screenresolution.GetPrimary()

var settings = shankskit.AppSettings{
	Port:        "8080",
	AppName:     "Chirp",
	Fullscreen:  false,
	Width:       500,
	Height:      resolution.Height,
	VisibleUI:   false,
	Transparent: true,
	AlwaysOnTop: true,
	Routes:      routes,
}

func main() {
	err := startOllamaServe() 	
	if err != nil {
		fmt.Println("Failed to start ollama serve:", err)
		return
	}
	appWindow, a, server := shankskit.StartApp(settings)
	go manageWindowPos(appWindow)
	appWindow.Move((resolution.Width-500)/2, 0)
	shankskit.HandleShutDown(a, server)
}

func chat(w http.ResponseWriter, r *http.Request) {
    shankskit.Respond(w, "pages/chat.html", nil)
}
func message(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse prompt from POST body (assumes JSON: { "prompt": "your text here" })
	var body struct {
		Prompt string `json:"prompt"`
	}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil || body.Prompt == "" {
		http.Error(w, "Invalid JSON or missing 'prompt'", http.StatusBadRequest)
		return
	}

	// Set headers for streaming response
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Stream Llama3 response directly to client
	err = StreamLlama3ResponseToWriter(body.Prompt, w, flusher)
	if err != nil {
		http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
	}
}

func startOllamaServe() error {
    cmd := exec.Command("ollama", "serve")

    // Optional: attach stdout/stderr if you want to see output
    // cmd.Stdout = os.Stdout
    // cmd.Stderr = os.Stderr

    // Hide console window on Windows
    cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

    // Start process asynchronously
    return cmd.Start()
}

func manageWindowPos(appWindow *astilectron.Window) {
	// Set the initial position of the window
	appWindow.Hide()
	for {
		_, y, _ := GetCursorPos()
		if y > resolution.Height-125 {
			appWindow.Show()
			for {
				_, y, _ = GetCursorPos()
				if y < resolution.Height-125 {
					appWindow.Hide()
					break
				}
				time.Sleep(100 * time.Millisecond) // Was 1s — too slow!
			}
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

type Point struct {
	X, Y int32
}

func GetCursorPos() (int, int, error) {
	var pt Point
	user32 := windows.NewLazySystemDLL("user32.dll")
	getCursorPos := user32.NewProc("GetCursorPos")

	ret, _, err := getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if ret == 0 {
		return 0, 0, err
	}
	return int(pt.X), int(pt.Y), nil
}

func StreamLlama3ResponseToWriter(prompt string, w http.ResponseWriter, flusher http.Flusher) error {
	type RequestBody struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}

	requestBody := RequestBody{
		Model:  "llama3",
		Prompt: prompt,
		Stream: true,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			return err
		}
		if token, ok := data["response"].(string); ok {
			_, err := w.Write([]byte(token))
			if err != nil {
				return err
			}
			flusher.Flush()
		}
	}
	return scanner.Err()
}



