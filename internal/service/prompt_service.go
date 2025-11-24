package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"noti/internal/domain"

	"github.com/google/uuid"
)

// PromptService handles prompt management operations
type PromptService struct {
	promptsPath string
}

// NewPromptService creates a new prompt service
func NewPromptService(basePath string) *PromptService {
	promptsPath := filepath.Join(basePath, "prompts")
	return &PromptService{
		promptsPath: promptsPath,
	}
}

// Initialize creates the prompts directory if it doesn't exist
func (s *PromptService) Initialize() error {
	if err := os.MkdirAll(s.promptsPath, 0755); err != nil {
		return fmt.Errorf("failed to create prompts directory: %w", err)
	}

	// Create default prompts if directory is empty
	prompts, err := s.GetAll()
	if err != nil {
		return err
	}

	if len(prompts) == 0 {
		if err := s.createDefaultPrompts(); err != nil {
			return fmt.Errorf("failed to create default prompts: %w", err)
		}
	}

	return nil
}

// GetAll returns all prompts
func (s *PromptService) GetAll() ([]domain.Prompt, error) {
	files, err := os.ReadDir(s.promptsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.Prompt{}, nil
		}
		return nil, fmt.Errorf("failed to read prompts directory: %w", err)
	}

	var prompts []domain.Prompt
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
			continue
		}

		prompt, err := s.loadPromptFromFile(filepath.Join(s.promptsPath, file.Name()))
		if err != nil {
			fmt.Printf("Warning: failed to load prompt %s: %v\n", file.Name(), err)
			continue
		}

		prompts = append(prompts, *prompt)
	}

	return prompts, nil
}

// Get returns a prompt by ID
func (s *PromptService) Get(id string) (*domain.Prompt, error) {
	filename := fmt.Sprintf("%s.md", id)
	filePath := filepath.Join(s.promptsPath, filename)

	return s.loadPromptFromFile(filePath)
}

// Create creates a new prompt
func (s *PromptService) Create(name, description, systemPrompt, userPrompt string, temperature float32, maxTokens int) (*domain.Prompt, error) {
	id := uuid.New().String()
	now := time.Now()

	prompt := &domain.Prompt{
		ID:           id,
		Name:         name,
		Description:  description,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  temperature,
		MaxTokens:    maxTokens,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.savePromptToFile(prompt); err != nil {
		return nil, err
	}

	return prompt, nil
}

// Update updates an existing prompt
func (s *PromptService) Update(id, name, description, systemPrompt, userPrompt string, temperature float32, maxTokens int) error {
	existing, err := s.Get(id)
	if err != nil {
		return err
	}

	existing.Name = name
	existing.Description = description
	existing.SystemPrompt = systemPrompt
	existing.UserPrompt = userPrompt
	existing.Temperature = temperature
	existing.MaxTokens = maxTokens
	existing.UpdatedAt = time.Now()

	return s.savePromptToFile(existing)
}

// Delete deletes a prompt
func (s *PromptService) Delete(id string) error {
	filename := fmt.Sprintf("%s.md", id)
	filePath := filepath.Join(s.promptsPath, filename)

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete prompt: %w", err)
	}

	return nil
}

// loadPromptFromFile loads a prompt from a markdown file
func (s *PromptService) loadPromptFromFile(filePath string) (*domain.Prompt, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file: %w", err)
	}

	content := string(data)

	// Parse markdown format
	prompt := &domain.Prompt{}

	// Extract metadata from frontmatter
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content[4:], "\n---\n", 2)
		if len(parts) == 2 {
			// Parse frontmatter
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(parts[0]), &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
			}

			prompt.ID = metadata["id"].(string)
			prompt.Name = metadata["name"].(string)
			if desc, ok := metadata["description"].(string); ok {
				prompt.Description = desc
			}
			if temp, ok := metadata["temperature"].(float64); ok {
				prompt.Temperature = float32(temp)
			}
			if tokens, ok := metadata["maxTokens"].(float64); ok {
				prompt.MaxTokens = int(tokens)
			}
			if created, ok := metadata["createdAt"].(string); ok {
				prompt.CreatedAt, _ = time.Parse(time.RFC3339, created)
			}
			if updated, ok := metadata["updatedAt"].(string); ok {
				prompt.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
			}

			// Parse body sections
			body := parts[1]
			sections := strings.Split(body, "\n## ")

			for _, section := range sections {
				section = strings.TrimSpace(section)
				if section == "" {
					continue
				}

				if strings.HasPrefix(section, "System Prompt\n") {
					prompt.SystemPrompt = strings.TrimSpace(strings.TrimPrefix(section, "System Prompt\n"))
				} else if strings.HasPrefix(section, "User Prompt\n") {
					prompt.UserPrompt = strings.TrimSpace(strings.TrimPrefix(section, "User Prompt\n"))
				}
			}
		}
	}

	return prompt, nil
}

// savePromptToFile saves a prompt to a markdown file
func (s *PromptService) savePromptToFile(prompt *domain.Prompt) error {
	filename := fmt.Sprintf("%s.md", prompt.ID)
	filePath := filepath.Join(s.promptsPath, filename)

	// Create frontmatter
	metadata := map[string]interface{}{
		"id":          prompt.ID,
		"name":        prompt.Name,
		"description": prompt.Description,
		"temperature": prompt.Temperature,
		"maxTokens":   prompt.MaxTokens,
		"createdAt":   prompt.CreatedAt.Format(time.RFC3339),
		"updatedAt":   prompt.UpdatedAt.Format(time.RFC3339),
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Build markdown content
	var content strings.Builder
	content.WriteString("---\n")
	content.Write(metadataJSON)
	content.WriteString("\n---\n\n")
	content.WriteString("## System Prompt\n\n")
	content.WriteString(prompt.SystemPrompt)
	content.WriteString("\n\n## User Prompt\n\n")
	content.WriteString(prompt.UserPrompt)
	content.WriteString("\n")

	if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	return nil
}

// createDefaultPrompts creates some default prompts
func (s *PromptService) createDefaultPrompts() error {
	defaultPrompts := []struct {
		name         string
		description  string
		systemPrompt string
		userPrompt   string
		temperature  float32
		maxTokens    int
	}{
		{
			name:         "Summarize",
			description:  "Create a concise summary of the note",
			systemPrompt: "You are a helpful assistant that creates clear, concise summaries.",
			userPrompt:   "Please summarize the following note in 3-5 bullet points:\n\n{{content}}",
			temperature:  0.3,
			maxTokens:    2048,
		},
		{
			name:         "Cleanup",
			description:  "Cleans up your note and write grammatically correct text.",
			systemPrompt: "You are a meticulous copyeditor. Your sole task is to take the provided raw text and correct it to strictly adhere to standard written English language rules.",
			userPrompt: `Goal: Produce a final text that is grammatically perfect and professionally formatted, while ensuring the meaning and vocabulary of the original text remain completely unchanged.
							Instructions:
							Grammar Correction: Fix all grammatical errors, including subject-verb agreement issues, tense inconsistencies, pronoun errors, and modifier misplacements.
							Punctuation and Capitalization: Add or adjust standard punctuation (periods, commas, semicolons, etc.) and correct capitalization to create distinct, well-formed sentences.
							Spelling: Correct any misspelled words.
							Standard Formatting: Ensure proper spacing and indentation where applicable (e.g., separating paragraphs).
							Strict Constraint: Do not delete, add, or substitute any words or phrases unless they are filler words (like uh, um) or misspellings. If a sentence is awkward but grammatically correct, leave it alone. The goal is to polish, not rephrase.
							Input Text (to be inserted below): {{content}}

							Output Format: Provide only the final, cleaned, and correctly formatted text.`,
			temperature: 0.2,
			maxTokens:   2048,
		},
		{
			name:         "Expand Ideas",
			description:  "Expand and elaborate on the ideas in the note",
			systemPrompt: "You are a creative thinking assistant that helps expand and develop ideas.",
			userPrompt:   "Please expand on the following ideas with additional details, examples, and perspectives:\n\n{{content}}",
			temperature:  0.7,
			maxTokens:    2048,
		},
		{
			name:         "Improve Writing",
			description:  "Improve the writing quality and clarity",
			systemPrompt: "You are an expert editor. Improve clarity, grammar, and style while maintaining the original meaning.",
			userPrompt:   "Please improve the writing of the following text:\n\n{{content}}",
			temperature:  0.5,
			maxTokens:    2048,
		},
		{
			name:         "Extract Action Items",
			description:  "Extract actionable tasks from the note",
			systemPrompt: "You are a productivity assistant that identifies actionable tasks.",
			userPrompt:   "Please extract all action items and tasks from the following note as a numbered list:\n\n{{content}}",
			temperature:  0.2,
			maxTokens:    2048,
		},
		{
			name:         "Generate Questions",
			description:  "Generate thought-provoking questions about the content",
			systemPrompt: "You are a curious assistant that asks insightful questions to deepen understanding.",
			userPrompt:   "Please generate 5-7 thought-provoking questions about the following content:\n\n{{content}}",
			temperature:  0.6,
			maxTokens:    2048,
		},
	}

	for _, p := range defaultPrompts {
		if _, err := s.Create(p.name, p.description, p.systemPrompt, p.userPrompt, p.temperature, p.maxTokens); err != nil {
			return err
		}
	}

	return nil
}
