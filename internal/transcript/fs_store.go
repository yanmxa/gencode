package transcript

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const transcriptIndexFile = "transcripts-index.json"

type FileStore struct {
	mu        sync.RWMutex
	baseDir   string
	projectID string
}

type fileIndex struct {
	Version   int              `json:"version"`
	ProjectID string           `json:"projectId"`
	Entries   []fileIndexEntry `json:"entries"`
}

type fileIndexEntry struct {
	TranscriptID string    `json:"transcriptId"`
	FullPath     string    `json:"fullPath"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Title        string    `json:"title,omitempty"`
	LastPrompt   string    `json:"lastPrompt,omitempty"`
	MessageCount int       `json:"messageCount"`
	GitBranch    string    `json:"gitBranch,omitempty"`
	HasSummary   bool      `json:"hasSummary,omitempty"`
	IsSidechain  bool      `json:"isSidechain,omitempty"`
}

func NewFileStore(baseDir, projectID string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Join(baseDir, "transcripts"), 0o755); err != nil {
		return nil, fmt.Errorf("create transcripts dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "blobs", "summary"), 0o755); err != nil {
		return nil, fmt.Errorf("create summary blobs dir: %w", err)
	}
	return &FileStore{baseDir: baseDir, projectID: projectID}, nil
}

func (s *FileStore) Start(ctx context.Context, cmd StartCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	path := s.transcriptPath(cmd.TranscriptID)
	exists, err := fileExists(path)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	rec := Record{
		ID:           cmd.TranscriptID + ":start",
		TranscriptID: cmd.TranscriptID,
		Time:         cmd.Time,
		Type:         RecordStarted,
		Cwd:          cmd.Cwd,
		System: &SystemRecord{
			Provider: cmd.Provider,
			Model:    cmd.Model,
			ParentID: cmd.ParentID,
		},
	}
	if err := s.appendRecord(path, rec); err != nil {
		return err
	}
	return s.refreshIndexLocked(cmd.TranscriptID)
}

func (s *FileStore) AppendMessage(ctx context.Context, cmd AppendMessageCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	path := s.transcriptPath(cmd.TranscriptID)
	exists, err := s.messageExistsLocked(path, cmd.MessageID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	rec := Record{
		ID:           cmd.TranscriptID + ":" + cmd.MessageID,
		TranscriptID: cmd.TranscriptID,
		Time:         cmd.Time,
		Type:         RecordMessageAppended,
		ParentID:     cmd.ParentID,
		Cwd:          cmd.Cwd,
		GitBranch:    cmd.GitBranch,
		AgentID:      cmd.AgentID,
		IsSidechain:  cmd.IsSidechain,
		Message: &MessageRecord{
			MessageID: cmd.MessageID,
			Role:      cmd.Role,
			Content:   append([]ContentBlock(nil), cmd.Content...),
		},
	}
	if err := s.appendRecord(path, rec); err != nil {
		return err
	}
	return s.refreshIndexLocked(cmd.TranscriptID)
}

func (s *FileStore) PatchState(ctx context.Context, cmd PatchStateCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	rec := Record{
		ID:           fmt.Sprintf("%s:state:%d", cmd.TranscriptID, cmd.Time.UnixNano()),
		TranscriptID: cmd.TranscriptID,
		Time:         cmd.Time,
		Type:         RecordStatePatched,
		State: &StateRecord{
			Ops: append([]PatchOp(nil), cmd.Ops...),
		},
	}
	if err := s.appendRecord(s.transcriptPath(cmd.TranscriptID), rec); err != nil {
		return err
	}
	return s.refreshIndexLocked(cmd.TranscriptID)
}

func (s *FileStore) Compact(ctx context.Context, cmd CompactCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	system := &SystemRecord{BoundaryID: cmd.BoundaryID}
	if cmd.Summary != "" {
		blobID := fmt.Sprintf("%s-%d", cmd.TranscriptID, cmd.Time.UnixNano())
		if err := os.WriteFile(s.summaryBlobPath(blobID), []byte(cmd.Summary), 0o644); err != nil {
			return fmt.Errorf("write summary blob: %w", err)
		}
		system.SummaryBlobID = blobID
	}

	rec := Record{
		ID:           fmt.Sprintf("%s:compact:%d", cmd.TranscriptID, cmd.Time.UnixNano()),
		TranscriptID: cmd.TranscriptID,
		Time:         cmd.Time,
		Type:         RecordCompacted,
		System:       system,
	}
	if err := s.appendRecord(s.transcriptPath(cmd.TranscriptID), rec); err != nil {
		return err
	}
	return s.refreshIndexLocked(cmd.TranscriptID)
}

func (s *FileStore) Fork(ctx context.Context, cmd ForkCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	sourcePath := s.transcriptPath(cmd.SourceTranscriptID)
	records, err := s.loadRecordsLocked(sourcePath)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("source transcript not found: %s", cmd.SourceTranscriptID)
	}

	destPath := s.transcriptPath(cmd.NewTranscriptID)
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open fork transcript: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, rec := range records {
		rec.TranscriptID = cmd.NewTranscriptID
		rec.Time = cmd.Time
		if err := enc.Encode(rec); err != nil {
			_ = f.Close()
			return fmt.Errorf("write fork record: %w", err)
		}
	}
	forkRec := Record{
		ID:           fmt.Sprintf("%s:fork:%d", cmd.NewTranscriptID, cmd.Time.UnixNano()),
		TranscriptID: cmd.NewTranscriptID,
		Time:         cmd.Time,
		Type:         RecordForked,
		System: &SystemRecord{
			ParentID: cmd.SourceTranscriptID,
		},
	}
	if err := enc.Encode(forkRec); err != nil {
		_ = f.Close()
		return fmt.Errorf("write fork marker: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close fork transcript: %w", err)
	}
	return s.refreshIndexLocked(cmd.NewTranscriptID)
}

func (s *FileStore) Replace(ctx context.Context, cmd ReplaceCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	tx := cmd.Transcript
	if tx.ID == "" {
		return fmt.Errorf("replace transcript: missing transcript ID")
	}

	records, err := recordsForTranscript(tx)
	if err != nil {
		return err
	}

	path := s.transcriptPath(tx.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open transcript file: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			_ = f.Close()
			return fmt.Errorf("write transcript record: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close transcript file: %w", err)
	}
	return s.refreshIndexLocked(tx.ID)
}

func (s *FileStore) Load(ctx context.Context, transcriptID string) (*Transcript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	records, err := s.loadRecordsLocked(s.transcriptPath(transcriptID))
	if err != nil {
		return nil, err
	}
	return Project(records, fileBlobReader{s: s})
}

func (s *FileStore) LoadLatest(ctx context.Context, projectID string) (*Transcript, error) {
	items, err := s.List(ctx, projectID, ListOptions{Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no transcripts found")
	}
	return s.Load(ctx, items[0].TranscriptID)
}

func (s *FileStore) List(ctx context.Context, projectID string, opts ListOptions) ([]ListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if projectID != "" && s.projectID != "" && projectID != s.projectID {
		return []ListItem{}, nil
	}

	index, err := s.loadIndexLocked()
	if err != nil {
		if err := s.rebuildIndexLocked(); err != nil {
			return nil, err
		}
		index, err = s.loadIndexLocked()
		if err != nil {
			return nil, err
		}
	}

	items := make([]ListItem, 0, len(index.Entries))
	for _, entry := range index.Entries {
		if !opts.IncludeSidechain && entry.IsSidechain {
			continue
		}
		items = append(items, ListItem{
			TranscriptID: entry.TranscriptID,
			FullPath:     entry.FullPath,
			CreatedAt:    entry.CreatedAt,
			UpdatedAt:    entry.UpdatedAt,
			Title:        entry.Title,
			LastPrompt:   entry.LastPrompt,
			MessageCount: entry.MessageCount,
			GitBranch:    entry.GitBranch,
			HasSummary:   entry.HasSummary,
			IsSidechain:  entry.IsSidechain,
		})
	}

	slices.SortFunc(items, func(a, b ListItem) int {
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}
	return items, nil
}

func (s *FileStore) RebuildIndex(ctx context.Context, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if projectID != "" && s.projectID != "" && projectID != s.projectID {
		return nil
	}
	return s.rebuildIndexLocked()
}

func (s *FileStore) Delete(ctx context.Context, transcriptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(s.transcriptPath(transcriptID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete transcript file: %w", err)
	}

	index, err := s.loadIndexLocked()
	if err == nil {
		filtered := make([]fileIndexEntry, 0, len(index.Entries))
		for _, entry := range index.Entries {
			if entry.TranscriptID != transcriptID {
				filtered = append(filtered, entry)
			}
		}
		index.Entries = filtered
		return s.saveIndexLocked(index)
	}
	return nil
}

type fileBlobReader struct {
	s *FileStore
}

func (r fileBlobReader) Get(kind, id string) ([]byte, error) {
	switch kind {
	case "summary":
		return os.ReadFile(r.s.summaryBlobPath(id))
	default:
		return nil, fmt.Errorf("unsupported blob kind: %s", kind)
	}
}

func (s *FileStore) transcriptPath(transcriptID string) string {
	return filepath.Join(s.baseDir, "transcripts", transcriptID+".jsonl")
}

func (s *FileStore) TranscriptPath(transcriptID string) string {
	return s.transcriptPath(transcriptID)
}

func (s *FileStore) summaryBlobPath(blobID string) string {
	return filepath.Join(s.baseDir, "blobs", "summary", blobID+".md")
}

func (s *FileStore) indexPath() string {
	return filepath.Join(s.baseDir, transcriptIndexFile)
}

func (s *FileStore) appendRecord(path string, rec Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transcript dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open transcript file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return fmt.Errorf("append transcript record: %w", err)
	}
	return nil
}

func recordsForTranscript(tx Transcript) ([]Record, error) {
	now := time.Now()
	createdAt := tx.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := tx.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	records := []Record{{
		ID:           tx.ID + ":start",
		TranscriptID: tx.ID,
		Time:         createdAt,
		Type:         RecordStarted,
		Cwd:          tx.Cwd,
		System: &SystemRecord{
			Provider: tx.Provider,
			Model:    tx.Model,
			ParentID: tx.ParentID,
		},
	}}

	for i, node := range tx.Messages {
		messageTime := node.Time
		if messageTime.IsZero() {
			messageTime = createdAt.Add(time.Duration(i+1) * time.Millisecond)
		}
		if node.ID == "" {
			return nil, fmt.Errorf("replace transcript: message %d missing ID", i)
		}
		records = append(records, Record{
			ID:           tx.ID + ":" + node.ID,
			TranscriptID: tx.ID,
			Time:         messageTime,
			Type:         RecordMessageAppended,
			ParentID:     node.ParentID,
			Cwd:          node.Cwd,
			GitBranch:    node.GitBranch,
			AgentID:      node.AgentID,
			IsSidechain:  node.IsSidechain,
			Message: &MessageRecord{
				MessageID: node.ID,
				Role:      node.Role,
				Content:   append([]ContentBlock(nil), node.Content...),
			},
		})
	}

	ops := []PatchOp{
		PatchTitle(tx.State.Title),
		PatchLastPrompt(tx.State.LastPrompt),
		PatchTag(tx.State.Tag),
		PatchMode(tx.State.Mode),
		PatchSummary(tx.State.Summary),
	}
	if len(tx.State.Tasks) > 0 {
		ops = append(ops, PatchTasks(TrackerTasksFromView(tx.State.Tasks)))
	}
	if tx.State.Worktree != nil {
		ops = append(ops, PatchWorktree(tx.State.Worktree))
	}
	records = append(records, Record{
		ID:           fmt.Sprintf("%s:state:%d", tx.ID, updatedAt.UnixNano()),
		TranscriptID: tx.ID,
		Time:         updatedAt,
		Type:         RecordStatePatched,
		State: &StateRecord{
			Ops: ops,
		},
	})
	return records, nil
}

func (s *FileStore) messageExistsLocked(path, messageID string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("open transcript file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		var rec Record
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		if rec.Type == RecordMessageAppended && rec.Message != nil && rec.Message.MessageID == messageID {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("scan transcript file: %w", err)
	}
	return false, nil
}

func (s *FileStore) loadRecordsLocked(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("open transcript file: %w", err)
		}
		return nil, fmt.Errorf("open transcript file: %w", err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("decode transcript record: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript file: %w", err)
	}
	return records, nil
}

func (s *FileStore) loadIndexLocked() (*fileIndex, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		return nil, err
	}
	var index fileIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

func (s *FileStore) saveIndexLocked(index *fileIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal transcript index: %w", err)
	}
	if err := os.WriteFile(s.indexPath(), data, 0o644); err != nil {
		return fmt.Errorf("write transcript index: %w", err)
	}
	return nil
}

func (s *FileStore) rebuildIndexLocked() error {
	entries, err := os.ReadDir(filepath.Join(s.baseDir, "transcripts"))
	if err != nil {
		return fmt.Errorf("read transcripts dir: %w", err)
	}

	index := &fileIndex{
		Version:   1,
		ProjectID: s.projectID,
		Entries:   make([]fileIndexEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		transcriptID := strings.TrimSuffix(entry.Name(), ".jsonl")
		item, err := s.buildListItemLocked(transcriptID)
		if err != nil {
			continue
		}
		index.Entries = append(index.Entries, fileIndexEntry{
			TranscriptID: item.TranscriptID,
			FullPath:     item.FullPath,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
			Title:        item.Title,
			LastPrompt:   item.LastPrompt,
			MessageCount: item.MessageCount,
			GitBranch:    item.GitBranch,
			HasSummary:   item.HasSummary,
			IsSidechain:  item.IsSidechain,
		})
	}
	return s.saveIndexLocked(index)
}

func (s *FileStore) refreshIndexLocked(transcriptID string) error {
	index, err := s.loadIndexLocked()
	if err != nil {
		index = &fileIndex{
			Version:   1,
			ProjectID: s.projectID,
		}
	}

	item, err := s.buildListItemLocked(transcriptID)
	if err != nil {
		return err
	}

	entry := fileIndexEntry{
		TranscriptID: item.TranscriptID,
		FullPath:     item.FullPath,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
		Title:        item.Title,
		LastPrompt:   item.LastPrompt,
		MessageCount: item.MessageCount,
		GitBranch:    item.GitBranch,
		HasSummary:   item.HasSummary,
		IsSidechain:  item.IsSidechain,
	}

	for i := range index.Entries {
		if index.Entries[i].TranscriptID == transcriptID {
			index.Entries[i] = entry
			return s.saveIndexLocked(index)
		}
	}
	index.Entries = append(index.Entries, entry)
	return s.saveIndexLocked(index)
}

func (s *FileStore) buildListItemLocked(transcriptID string) (ListItem, error) {
	records, err := s.loadRecordsLocked(s.transcriptPath(transcriptID))
	if err != nil {
		return ListItem{}, err
	}
	transcript, err := Project(records, fileBlobReader{s: s})
	if err != nil {
		return ListItem{}, err
	}

	title := transcript.State.Title
	if title == "" {
		title = firstUserText(transcript.Messages)
	}

	return ListItem{
		TranscriptID: transcriptID,
		FullPath:     s.transcriptPath(transcriptID),
		CreatedAt:    transcript.CreatedAt,
		UpdatedAt:    transcript.UpdatedAt,
		Title:        title,
		LastPrompt:   coalesce(transcript.State.LastPrompt, lastUserText(transcript.Messages)),
		MessageCount: len(transcript.Messages),
		GitBranch:    lastGitBranch(transcript.Messages),
		HasSummary:   transcript.State.Summary != "",
		IsSidechain:  anySidechain(transcript.Messages),
	}, nil
}

func firstUserText(messages []Node) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		if text := firstTextBlock(msg.Content); text != "" {
			return text
		}
	}
	return ""
}

func lastUserText(messages []Node) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if text := firstTextBlock(messages[i].Content); text != "" {
			return text
		}
	}
	return ""
}

func firstTextBlock(content []ContentBlock) string {
	for _, block := range content {
		if block.Type == "tool_result" {
			return ""
		}
		if block.Type == "text" && block.Text != "" {
			return block.Text
		}
	}
	return ""
}

func lastGitBranch(messages []Node) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].GitBranch != "" {
			return messages[i].GitBranch
		}
	}
	return ""
}

func anySidechain(messages []Node) bool {
	for _, msg := range messages {
		if msg.IsSidechain {
			return true
		}
	}
	return false
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
