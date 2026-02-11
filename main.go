package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/google/uuid"
	openrouter "github.com/revrost/go-openrouter"

	"github.com/andys/orchat/storage"
)

var (
	store    *storage.Store
	orClient *openrouter.Client
)

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}

	dataDir := os.Getenv("ORCHAT_DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	var err error
	store, err = storage.New(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	orClient = openrouter.NewClient(
		apiKey,
		openrouter.WithXTitle("OrChat"),
	)

	app := fiber.New(fiber.Config{
		BodyLimit: 10 * 1024 * 1024, // 10MB
	})

	// API routes
	api := app.Group("/api")
	api.Get("/models", handleListModels)
	api.Get("/chats", handleListChats)
	api.Post("/chats", handleCreateChat)
	api.Get("/chats/:id", handleGetChat)
	api.Delete("/chats/:id", handleDeleteChat)
	api.Post("/chats/:id/messages", handleSendMessage)
	api.Post("/render-markdown", handleRenderMarkdown)
	api.Get("/preferences", handleGetPreferences)
	api.Put("/preferences", handleSavePreferences)

	// Serve frontend
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("static/index.html")
	})
	app.Static("/static", "./static")

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("OrChat listening on http://localhost:%s", port)
	log.Fatal(app.Listen(":" + port))
}

// handleListModels fetches available models from OpenRouter.
func handleListModels(c *fiber.Ctx) error {
	models, err := orClient.ListModels(context.Background())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("list models: %v", err)})
	}

	// Filter to text-capable models and sort by name
	type modelInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	var result []modelInfo
	for _, m := range models {
		// Only include models that can output text
		hasTextOutput := false
		for _, mod := range m.Architecture.OutputModalities {
			if mod == "text" {
				hasTextOutput = true
				break
			}
		}
		if !hasTextOutput {
			continue
		}
		result = append(result, modelInfo{ID: m.ID, Name: m.Name})
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return c.JSON(result)
}

// handleListChats returns all chat metadata.
func handleListChats(c *fiber.Ctx) error {
	chats, err := store.ListChats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("list chats: %v", err)})
	}
	if chats == nil {
		chats = []storage.Chat{}
	}
	return c.JSON(chats)
}

// handleCreateChat creates a new empty chat.
func handleCreateChat(c *fiber.Ctx) error {
	var body struct {
		Model string `json:"model"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	now := time.Now().Unix()
	chat := &storage.Chat{
		ID:        uuid.New().String(),
		Title:     "New Chat",
		Model:     body.Model,
		Messages:  []storage.Message{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.SaveChat(chat); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("save chat: %v", err)})
	}

	return c.Status(201).JSON(chat)
}

// renderMarkdownToHTML converts markdown text to HTML.
func renderMarkdownToHTML(text string) string {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock | parser.FencedCode
	p := parser.NewWithExtensions(extensions)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	doc := p.Parse([]byte(text))
	return string(markdown.Render(doc, renderer))
}

// handleRenderMarkdown accepts markdown text and returns rendered HTML.
func handleRenderMarkdown(c *fiber.Ctx) error {
	var body struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	rendered := renderMarkdownToHTML(body.Content)
	return c.JSON(fiber.Map{"html": rendered})
}

// messageResponse is a Message with an optional content_html field for assistant messages.
type messageResponse struct {
	Role        string `json:"role"`
	Content     string `json:"content"`
	ContentHTML string `json:"content_html,omitempty"`
	Timestamp   int64  `json:"timestamp"`
}

// chatResponse is a Chat response with rendered HTML for assistant messages.
type chatResponse struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Model     string            `json:"model"`
	Messages  []messageResponse `json:"messages"`
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
}

// handleGetChat returns a chat with all messages, including rendered HTML for assistant messages.
func handleGetChat(c *fiber.Ctx) error {
	id := c.Params("id")
	chat, err := store.LoadChat(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "chat not found"})
	}

	resp := chatResponse{
		ID:        chat.ID,
		Title:     chat.Title,
		Model:     chat.Model,
		CreatedAt: chat.CreatedAt,
		UpdatedAt: chat.UpdatedAt,
	}

	for _, msg := range chat.Messages {
		mr := messageResponse{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
		if msg.Role == "assistant" {
			mr.ContentHTML = renderMarkdownToHTML(msg.Content)
		}
		resp.Messages = append(resp.Messages, mr)
	}
	if resp.Messages == nil {
		resp.Messages = []messageResponse{}
	}

	return c.JSON(resp)
}

// handleDeleteChat removes a chat.
func handleDeleteChat(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := store.DeleteChat(id); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "chat not found"})
	}
	return c.SendStatus(204)
}

// handleSendMessage sends a user message to the LLM and streams the response.
func handleSendMessage(c *fiber.Ctx) error {
	id := c.Params("id")
	chat, err := store.LoadChat(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "chat not found"})
	}

	var body struct {
		Content string `json:"content"`
		Model   string `json:"model"`
	}
	if err := c.BodyParser(&body); err != nil || body.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content is required"})
	}

	// Update model if changed
	if body.Model != "" {
		chat.Model = body.Model
	}

	// Add user message
	userMsg := storage.Message{
		Role:      "user",
		Content:   body.Content,
		Timestamp: time.Now().Unix(),
	}
	chat.Messages = append(chat.Messages, userMsg)

	// Auto-generate title from first user message
	if len(chat.Messages) == 1 {
		title := body.Content
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		chat.Title = title
	}

	// Build OpenRouter messages
	var orMessages []openrouter.ChatCompletionMessage
	for _, msg := range chat.Messages {
		orMessages = append(orMessages, openrouter.ChatCompletionMessage{
			Role: msg.Role,
			Content: openrouter.Content{
				Text: msg.Content,
			},
		})
	}

	// Set up SSE streaming
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ctx := context.Background()
		stream, err := orClient.CreateChatCompletionStream(ctx, openrouter.ChatCompletionRequest{
			Model:    chat.Model,
			Messages: orMessages,
			Stream:   true,
		})
		if err != nil {
			errData, _ := json.Marshal(fiber.Map{"error": err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", errData)
			w.Flush()
			return
		}
		defer stream.Close()

		var fullContent strings.Builder

		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				errData, _ := json.Marshal(fiber.Map{"error": err.Error()})
				fmt.Fprintf(w, "data: %s\n\n", errData)
				w.Flush()
				break
			}

			if len(response.Choices) > 0 {
				delta := response.Choices[0].Delta.Content
				if delta != "" {
					fullContent.WriteString(delta)
					chunk, _ := json.Marshal(fiber.Map{"content": delta})
					fmt.Fprintf(w, "data: %s\n\n", chunk)
					w.Flush()
				}
			}
		}

		// Save assistant message
		assistantMsg := storage.Message{
			Role:      "assistant",
			Content:   fullContent.String(),
			Timestamp: time.Now().Unix(),
		}
		chat.Messages = append(chat.Messages, assistantMsg)
		store.SaveChat(chat)

		// Send done event
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.Flush()
	})

	return nil
}

// handleGetPreferences returns user preferences.
func handleGetPreferences(c *fiber.Ctx) error {
	prefs, err := store.LoadPreferences()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("load preferences: %v", err)})
	}
	return c.JSON(prefs)
}

// handleSavePreferences saves user preferences.
func handleSavePreferences(c *fiber.Ctx) error {
	var prefs storage.Preferences
	if err := c.BodyParser(&prefs); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := store.SavePreferences(&prefs); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("save preferences: %v", err)})
	}
	return c.JSON(prefs)
}
