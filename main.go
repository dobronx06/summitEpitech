package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	baseURL    = "http://localhost:1234/v1"
	modelName  = "llama-3.2-1b-instruct"
	apiTimeout = 120 * time.Second
)

type CodeFixRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Tools          []Tool    `json:"tools,omitempty"`
	ResponseFormat struct {
		Type       string     `json:"type"`
		JSONSchema JSONSchema `json:"json_schema,omitempty"`
	} `json:"response_format,omitempty"`
	Temperature float32 `json:"temperature"`
	Stream      bool    `json:"stream"`
}

type JSONSchema struct {
	Schema struct {
		Type       string              `json:"type"`
		Properties map[string]Property `json:"properties"`
		Required   []string            `json:"required"`
	} `json:"schema"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type CodeFixResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

type CodeFix struct {
	OriginalCode string `json:"original_code"`
	FixedCode    string `json:"fixed_code"`
	Explanation  string `json:"explanation"`
	Language     string `json:"language"`
	ErrorType    string `json:"error_type"`
}

func main() {
	if !checkServerAvailable() {
		fmt.Println("LM Studio server not available. Please ensure it's running at", baseURL)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: codefixer <filename>")
		os.Exit(1)
	}

	filename := os.Args[1]
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	fix, err := analyzeAndFixCode(string(content))
	if err != nil {
		fmt.Printf("Error fixing code: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Code Fix Report ===")
	fmt.Printf("Language: %s\nError Type: %s\n", fix.Language, fix.ErrorType)
	fmt.Println("\nOriginal Code:")
	fmt.Println(fix.OriginalCode)
	fmt.Println("\nFixed Code:")
	fmt.Println(fix.FixedCode)
	fmt.Println("\nExplanation:")
	fmt.Println(fix.Explanation)

	if err := validateAndSave(filename, fix); err != nil {
		fmt.Printf("\nError: %v\n", err)
	} else {
		fmt.Println("\nUpdate successful!")
	}
}

func analyzeAndFixCode(code string) (*CodeFix, error) {
	tools := []Tool{
		{
			Type: "function",
			Function: Function{
				Name:        "perform_code_analysis",
				Description: "Analyzes code for errors and suggests fixes",
				Parameters: Parameters{
					Type: "object",
					Properties: map[string]Property{
						"code": {
							Type:        "string",
							Description: "The code to analyze",
						},
						"language": {
							Type:        "string",
							Description: "Programming language of the code",
							Enum:        []string{"go", "python", "javascript", "java", "c++"},
						},
					},
					Required: []string{"code", "language"},
				},
			},
		},
	}

	schema := JSONSchema{}
	schema.Schema.Type = "object"
	schema.Schema.Properties = map[string]Property{
		"original_code": {Type: "string"},
		"fixed_code":    {Type: "string"},
		"explanation":   {Type: "string"},
		"language":      {Type: "string"},
		"error_type":    {Type: "string"},
	}
	schema.Schema.Required = []string{"fixed_code", "explanation", "language", "error_type"}

	messages := []Message{
		{
			Role: "system",
			Content: "You are an expert code debugging assistant. Analyze the provided code, " +
				"identify errors, and provide a corrected version with explanations. " +
				"Use structured JSON output format.",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Analyze and fix this code:\n```\n%s\n```", code),
		},
	}

	request := CodeFixRequest{
		Model:    modelName,
		Messages: messages,
		Tools:    tools,
		ResponseFormat: struct {
			Type       string     `json:"type"`
			JSONSchema JSONSchema `json:"json_schema,omitempty"`
		}{
			Type:       "json_schema",
			JSONSchema: schema,
		},
		Temperature: 0.3,
		Stream:      false,
	}

	response, err := sendChatRequest(request)
	if err != nil {
		return nil, err
	}

	var codeFix CodeFix
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &codeFix); err != nil {
		return nil, fmt.Errorf("error parsing JSON response: %v", err)
	}

	codeFix.OriginalCode = code
	return &codeFix, nil
}

func sendChatRequest(request CodeFixRequest) (*CodeFixResponse, error) {
	client := &http.Client{Timeout: apiTimeout}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed: %s\nResponse: %s", resp.Status, body)
	}

	var response CodeFixResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

func checkServerAvailable() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func validateAndSave(originalFile string, fix *CodeFix) error {
	fmt.Print("\nApply these changes? [y/N]: ")

	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("error opening terminal: %v", err)
	}
	defer tty.Close()

	var confirm string
	_, err = fmt.Fscanln(tty, &confirm)
	if err != nil && err != io.EOF {
		return fmt.Errorf("error reading input: %v", err)
	}

	if strings.ToLower(confirm) != "y" {
		return fmt.Errorf("user cancelled the operation")
	}

	// Create backup with timestamp
	backupName := fmt.Sprintf("%s.%s.bak", originalFile, time.Now().Format("20060102150405"))
	if err := os.Rename(originalFile, backupName); err != nil {
		return fmt.Errorf("error creating backup: %v", err)
	}

	// Write fixed content to original filename
	if err := os.WriteFile(originalFile, []byte(fix.FixedCode), 0644); err != nil {
		return fmt.Errorf("error writing fixed code: %v", err)
	}

	fmt.Printf("\nBackup saved to %s\n", backupName)

	// Validate the fixed code
	if fix.Language == "go" && strings.HasSuffix(originalFile, ".go") {
		fmt.Println("\nValidating Go code...")
		cmd := exec.Command("go", "build", "-o", "/dev/null", originalFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build failed: %s\n%s", err, string(output))
		}
		fmt.Println("Code compiled successfully!")
	}

	return nil
}