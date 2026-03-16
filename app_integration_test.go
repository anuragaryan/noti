package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/service"
	"noti/internal/stt/whisper"
)

type fakeLLMProvider struct {
	mu              sync.Mutex
	available       bool
	supportsStream  bool
	lastStreamReq   *domain.LLMRequest
	streamStartedCh chan struct{}
	streamFunc      func(ctx context.Context, req *domain.LLMRequest, callback domain.StreamCallback) error
}

func (f *fakeLLMProvider) SetContext(ctx context.Context) {}
func (f *fakeLLMProvider) Initialize() error              { return nil }
func (f *fakeLLMProvider) Cleanup()                       {}
func (f *fakeLLMProvider) GetModelInfo() map[string]interface{} {
	return map[string]interface{}{"provider": "fake"}
}
func (f *fakeLLMProvider) IsAvailable() bool       { return f.available }
func (f *fakeLLMProvider) SupportsStreaming() bool { return f.supportsStream }

func (f *fakeLLMProvider) Generate(ctx context.Context, request *domain.LLMRequest) (*domain.LLMResponse, error) {
	return &domain.LLMResponse{Text: "ok", Model: "fake", FinishReason: "stop"}, nil
}

func (f *fakeLLMProvider) GenerateStream(ctx context.Context, req *domain.LLMRequest, callback domain.StreamCallback) error {
	f.mu.Lock()
	f.lastStreamReq = req
	startedCh := f.streamStartedCh
	streamFn := f.streamFunc
	f.mu.Unlock()

	if startedCh != nil {
		select {
		case startedCh <- struct{}{}:
		default:
		}
	}

	if streamFn != nil {
		return streamFn(ctx, req, callback)
	}

	if err := callback(&domain.StreamChunk{Text: "chunk", Index: 0}); err != nil {
		return err
	}
	return callback(&domain.StreamChunk{Done: true, FinishReason: "stop", Index: 1})
}

func newIntegrationApp(t *testing.T) *App {
	t.Helper()

	tmp := t.TempDir()
	basePath := filepath.Join(tmp, "config")
	notesPath := filepath.Join(tmp, "notes")
	structurePath := filepath.Join(notesPath, "structure.json")

	structureRepo := repository.NewStructureRepository(structurePath)
	pathResolver := repository.NewPathResolver(notesPath)
	fileSystem := repository.NewFileSystem(pathResolver)

	folderService := service.NewFolderService(structureRepo, pathResolver, notesPath)
	noteService := service.NewNoteService(structureRepo, pathResolver, fileSystem, notesPath)
	configService := service.NewConfigService(basePath, defaultConfig)
	assetsService := service.NewAssetsService(basePath, defaultAssetsCatalog)
	sttManager := service.NewSTTManager(basePath)
	llmManager := service.NewLLMManager(basePath)
	promptService := service.NewPromptService(basePath)
	audioManager := service.NewAudioManager()
	sttManager.SetAudioManager(audioManager)

	if err := os.MkdirAll(notesPath, 0o755); err != nil {
		t.Fatalf("mkdir notes path: %v", err)
	}
	if err := structureRepo.Save(&domain.FolderStructure{Folders: []domain.Folder{}, Notes: []domain.Note{}}); err != nil {
		t.Fatalf("seed structure repo: %v", err)
	}

	config, err := configService.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := promptService.Initialize(); err != nil {
		t.Fatalf("init prompt service: %v", err)
	}

	app := &App{
		ctx:           context.Background(),
		eventsEnabled: false,
		basePath:      basePath,
		structurePath: structurePath,
		notesPath:     notesPath,
		config:        config,
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		fileSystem:    fileSystem,
		folderService: folderService,
		noteService:   noteService,
		configService: configService,
		assetsService: assetsService,
		sttManager:    sttManager,
		llmManager:    llmManager,
		promptService: promptService,
		audioManager:  audioManager,
	}

	setSTTManagerAvailable(t, app.sttManager, true)
	setLLMManagerProvider(t, app.llmManager, &fakeLLMProvider{available: true, supportsStream: true}, &app.config.LLM)

	return app
}

func setUnexportedField(t *testing.T, target interface{}, fieldName string, value interface{}) {
	t.Helper()
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		t.Fatalf("target must be a non-nil pointer")
	}
	e := v.Elem()
	f := e.FieldByName(fieldName)
	if !f.IsValid() {
		t.Fatalf("field %q not found", fieldName)
	}

	w := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	if value == nil {
		w.Set(reflect.Zero(f.Type()))
		return
	}

	val := reflect.ValueOf(value)
	if !val.Type().AssignableTo(f.Type()) {
		t.Fatalf("value type %s is not assignable to field %s (%s)", val.Type(), fieldName, f.Type())
	}
	w.Set(val)
}

func setSTTManagerAvailable(t *testing.T, manager *service.STTManager, available bool) {
	t.Helper()
	if available {
		setUnexportedField(t, manager, "transcriber", &whisper.Transcriber{})
		return
	}
	setUnexportedField(t, manager, "transcriber", nil)
}

func setLLMManagerProvider(t *testing.T, manager *service.LLMManager, provider service.LLMProvider, config *domain.LLMConfig) {
	t.Helper()
	setUnexportedField(t, manager, "provider", provider)
	if config != nil {
		cfgCopy := *config
		setUnexportedField(t, manager, "config", &cfgCopy)
	}
}

func TestNormalizeModelsEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "empty", in: "", wantErr: "API endpoint is required"},
		{name: "host only", in: "https://example.com", want: "https://example.com/v1/models"},
		{name: "chat completions", in: "https://example.com/v1/chat/completions", want: "https://example.com/v1/models"},
		{name: "already models", in: "https://example.com/v1/models", want: "https://example.com/v1/models"},
		{name: "custom path", in: "https://example.com/openai", want: "https://example.com/openai/v1/models"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeModelsEndpoint(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize endpoint: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected endpoint: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestGetAPILLMModels_SortsDedupesAndSetsHeaders(t *testing.T) {
	app := newIntegrationApp(t)

	var authHeader string
	var apiKeyHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		apiKeyHeader = r.Header.Get("api-key")
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"zeta"},{"id":"Alpha"},{"id":"zeta"},{"id":""}]}`))
	}))
	defer server.Close()

	models, err := app.GetAPILLMModels(server.URL+"/v1/chat/completions", "token")
	if err != nil {
		t.Fatalf("GetAPILLMModels: %v", err)
	}

	if authHeader != "Bearer token" {
		t.Fatalf("unexpected auth header: %q", authHeader)
	}
	if apiKeyHeader != "token" {
		t.Fatalf("unexpected api-key header: %q", apiKeyHeader)
	}

	if len(models) != 2 {
		t.Fatalf("unexpected model count: got %d", len(models))
	}
	if models[0].Code != "Alpha" || models[1].Code != "zeta" {
		t.Fatalf("unexpected model ordering: %#v", models)
	}
}

func TestGetAPILLMModels_PropagatesAPIErrorMessage(t *testing.T) {
	app := newIntegrationApp(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	_, err := app.GetAPILLMModels(server.URL, "bad")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("expected API error message in %q", err)
	}
}

func TestSaveConfig_PersistsWhenProviderAndModelUnchanged(t *testing.T) {
	app := newIntegrationApp(t)

	next := *app.config
	next.LLM.Temperature = 0.9
	next.Audio.Mixer.MicrophoneGain = 1.4
	next.Audio.Mixer.SystemGain = 0.7

	if err := app.SaveConfig(next); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if app.config.LLM.Temperature != 0.9 {
		t.Fatalf("expected in-memory config to be updated")
	}

	stored, err := app.configService.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if stored.LLM.Temperature != 0.9 || stored.Audio.Mixer.SystemGain != 0.7 {
		t.Fatalf("config was not persisted: %#v", stored)
	}
}

func TestSaveConfig_ValidatesMaxTokens(t *testing.T) {
	app := newIntegrationApp(t)
	next := *app.config
	next.LLM.MaxTokens = 10

	err := app.SaveConfig(next)
	if err == nil || !strings.Contains(err.Error(), "max tokens") {
		t.Fatalf("expected max tokens validation error, got %v", err)
	}
}

func TestExecutePromptOnNoteStream_InterpolatesNoteContent(t *testing.T) {
	app := newIntegrationApp(t)

	fakeProvider := &fakeLLMProvider{
		available:       true,
		supportsStream:  true,
		streamStartedCh: make(chan struct{}, 1),
		streamFunc: func(ctx context.Context, req *domain.LLMRequest, callback domain.StreamCallback) error {
			if err := callback(&domain.StreamChunk{Text: "ok", Index: 0}); err != nil {
				return err
			}
			return callback(&domain.StreamChunk{Done: true, FinishReason: "stop", Index: 1})
		},
	}
	setLLMManagerProvider(t, app.llmManager, fakeProvider, &app.config.LLM)

	note, err := app.CreateNote("Integration", "this is note content", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	prompt, err := app.CreatePrompt("Rewrite", "desc", "system prompt", "Rewrite this: {{content}}")
	if err != nil {
		t.Fatalf("CreatePrompt: %v", err)
	}

	if err := app.ExecutePromptOnNoteStream(prompt.ID, note.ID); err != nil {
		t.Fatalf("ExecutePromptOnNoteStream: %v", err)
	}

	select {
	case <-fakeProvider.streamStartedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not start")
	}

	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()
	if fakeProvider.lastStreamReq == nil {
		t.Fatalf("expected stream request to be captured")
	}
	if fakeProvider.lastStreamReq.SystemPrompt != "system prompt" {
		t.Fatalf("unexpected system prompt: %q", fakeProvider.lastStreamReq.SystemPrompt)
	}
	if !strings.Contains(fakeProvider.lastStreamReq.Prompt, "this is note content") {
		t.Fatalf("expected note content interpolation, got %q", fakeProvider.lastStreamReq.Prompt)
	}
}

func TestNotesFoldersFlow_CreateMoveSearchAndDelete(t *testing.T) {
	app := newIntegrationApp(t)

	folder, err := app.CreateFolder("Projects", "")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	note, err := app.CreateNote("Roadmap", "find me by keyword zebra", folder.ID)
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	if err := app.MoveNote(note.ID, ""); err != nil {
		t.Fatalf("MoveNote: %v", err)
	}

	if err := app.UpdateNote(note.ID, "Roadmap Updated", "content with zebra and llama", ""); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	results, err := app.SearchNotes("zebra", 10)
	if err != nil {
		t.Fatalf("SearchNotes: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one search result")
	}

	if err := app.DeleteFolder(folder.ID, false); err != nil {
		t.Fatalf("DeleteFolder preserve notes: %v", err)
	}

	got, err := app.GetNote(note.ID)
	if err != nil {
		t.Fatalf("GetNote after delete folder: %v", err)
	}
	if got.FolderID != "" {
		t.Fatalf("expected note to be moved to root, got folder=%q", got.FolderID)
	}
}

func TestGenerateTextStream_CancelStopsInFlightProvider(t *testing.T) {
	app := newIntegrationApp(t)

	canceled := make(chan struct{}, 1)
	provider := &fakeLLMProvider{
		available:      true,
		supportsStream: true,
		streamFunc: func(ctx context.Context, req *domain.LLMRequest, callback domain.StreamCallback) error {
			<-ctx.Done()
			canceled <- struct{}{}
			return ctx.Err()
		},
	}
	setLLMManagerProvider(t, app.llmManager, provider, &app.config.LLM)

	if err := app.GenerateTextStream("hello", ""); err != nil {
		t.Fatalf("GenerateTextStream: %v", err)
	}

	app.StopTextStream()

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected provider stream context to be canceled")
	}
}

func TestSaveConfig_ReturnsErrorWhenLLMSwitchFails(t *testing.T) {
	app := newIntegrationApp(t)

	setLLMManagerProvider(t, app.llmManager, nil, nil)
	next := *app.config
	next.LLM.Provider = "invalid-provider"

	err := app.SaveConfig(next)
	if err == nil {
		t.Fatal("expected llm switch failure")
	}
	if !strings.Contains(err.Error(), "failed to reinitialize LLM provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveConfig_WritesConfigJSON(t *testing.T) {
	app := newIntegrationApp(t)

	next := *app.config
	next.STTLanguage = "fr"
	next.Audio.DefaultSource = "system"

	if err := app.SaveConfig(next); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(app.basePath, "config.json"))
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode config json: %v", err)
	}
	if decoded["sttLanguage"] != "fr" {
		t.Fatalf("expected sttLanguage=fr, got %v", decoded["sttLanguage"])
	}

	audio, ok := decoded["audio"].(map[string]interface{})
	if !ok {
		t.Fatalf("audio not found in config")
	}
	if audio["defaultSource"] != "system" {
		t.Fatalf("expected defaultSource=system, got %v", audio["defaultSource"])
	}
}

func TestGetAPILLMModels_InvalidJSONErrorIncludesPreview(t *testing.T) {
	app := newIntegrationApp(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json-response`))
	}))
	defer server.Close()

	_, err := app.GetAPILLMModels(server.URL, "")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "response preview") {
		t.Fatalf("expected response preview in error, got %v", err)
	}
}

func TestGetAPILLMModels_RequiresEndpoint(t *testing.T) {
	app := newIntegrationApp(t)
	_, err := app.GetAPILLMModels("", "")
	if err == nil || !strings.Contains(err.Error(), "API endpoint is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveConfig_RejectsInvalidAudioMixerValues(t *testing.T) {
	app := newIntegrationApp(t)
	next := *app.config
	next.Audio.Mixer.SystemGain = 2.5

	err := app.SaveConfig(next)
	if err == nil {
		t.Fatal("expected invalid system gain error")
	}
	if !strings.Contains(err.Error(), "system gain") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAPILLMModels_HandlesTruncatedErrorBody(t *testing.T) {
	app := newIntegrationApp(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(strings.Repeat("x", 400)))
	}))
	defer server.Close()

	_, err := app.GetAPILLMModels(server.URL, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "...") {
		t.Fatalf("expected truncated preview in error, got %v", err)
	}
}

func TestSaveConfig_UsesCurrentConfigAsBaseline(t *testing.T) {
	app := newIntegrationApp(t)

	if app.config == nil {
		t.Fatal("expected initial config")
	}

	next := *app.config
	next.LLM.MaxTokens = app.config.LLM.MaxTokens + 128

	if err := app.SaveConfig(next); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if app.config.LLM.MaxTokens != next.LLM.MaxTokens {
		t.Fatalf("expected max tokens to update, got=%d want=%d", app.config.LLM.MaxTokens, next.LLM.MaxTokens)
	}
}

func TestGetAPILLMModels_PreservesBearerPrefix(t *testing.T) {
	app := newIntegrationApp(t)

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	_, err := app.GetAPILLMModels(server.URL, "Bearer abc")
	if err != nil {
		t.Fatalf("GetAPILLMModels: %v", err)
	}
	if authHeader != "Bearer abc" {
		t.Fatalf("expected existing bearer prefix to be preserved, got %q", authHeader)
	}
}

func TestGenerateTextStream_SecondCallCancelsFirst(t *testing.T) {
	app := newIntegrationApp(t)

	firstCanceled := make(chan struct{}, 1)
	provider := &fakeLLMProvider{
		available:      true,
		supportsStream: true,
		streamFunc: func(ctx context.Context, req *domain.LLMRequest, callback domain.StreamCallback) error {
			if req.Prompt == "first" {
				<-ctx.Done()
				firstCanceled <- struct{}{}
				return ctx.Err()
			}
			return callback(&domain.StreamChunk{Done: true, FinishReason: "stop"})
		},
	}
	setLLMManagerProvider(t, app.llmManager, provider, &app.config.LLM)

	if err := app.GenerateTextStream("first", ""); err != nil {
		t.Fatalf("start first stream: %v", err)
	}
	if err := app.GenerateTextStream("second", ""); err != nil {
		t.Fatalf("start second stream: %v", err)
	}

	select {
	case <-firstCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first stream to be canceled")
	}
}

func TestExecutePromptOnNoteStream_ReturnsErrorForUnknownPrompt(t *testing.T) {
	app := newIntegrationApp(t)
	note, err := app.CreateNote("Hello", "content", "")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	err = app.ExecutePromptOnNoteStream("missing", note.ID)
	if err == nil || !strings.Contains(err.Error(), "failed to get prompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAPILLMModels_WithWhitespaceEndpoint(t *testing.T) {
	app := newIntegrationApp(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
	}))
	defer server.Close()

	models, err := app.GetAPILLMModels(fmt.Sprintf("  %s  ", server.URL), "")
	if err != nil {
		t.Fatalf("GetAPILLMModels: %v", err)
	}
	if len(models) != 1 || models[0].Code != "model-a" {
		t.Fatalf("unexpected models: %#v", models)
	}
}
