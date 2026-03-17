package service

import (
	"testing"

	"noti/internal/domain"
)

func newMixedTestManager(chunkSize int) *AudioManager {
	m := NewAudioManager()
	m.mixerConfig = domain.DefaultMixerConfig()
	m.micBuffer = NewAudioRingBuffer(chunkSize * 4)
	m.sysBuffer = NewAudioRingBuffer(chunkSize * 4)
	return m
}

func TestMixNextChunk_EmitsWhenOnlyMicHasData(t *testing.T) {
	const chunkSize = 8
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.micBuffer.Write([]float32{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected first solo mic tick to wait for sync grace")
	}

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected mixed chunk after sustained missing system audio")
	}

	if len(mixed) != chunkSize {
		t.Fatalf("expected chunk size %d, got %d", chunkSize, len(mixed))
	}

	for i, sample := range mixed {
		if sample != 0.5 {
			t.Fatalf("expected sample %d to be 0.5, got %f", i, sample)
		}
	}
}

func TestMixNextChunk_EmitsWhenOnlySystemHasData(t *testing.T) {
	const chunkSize = 8
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.sysBuffer.Write([]float32{0.25, 0.25, 0.25, 0.25, 0.25, 0.25, 0.25, 0.25})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected first solo system tick to wait for sync grace")
	}

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected mixed chunk after sustained missing microphone audio")
	}

	if len(mixed) != chunkSize {
		t.Fatalf("expected chunk size %d, got %d", chunkSize, len(mixed))
	}

	for i, sample := range mixed {
		if sample != 0.25 {
			t.Fatalf("expected sample %d to be 0.25, got %f", i, sample)
		}
	}
}

func TestMixNextChunk_ReturnsFalseWhenInsufficientData(t *testing.T) {
	const chunkSize = 8
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.micBuffer.Write([]float32{0.1, 0.2, 0.3})
	m.sysBuffer.Write([]float32{0.1, 0.2})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected no chunk when neither source has enough samples")
	}
}

func TestMixNextChunk_EmitsAlignedWhenBothSourcesReady(t *testing.T) {
	const chunkSize = 4
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.micBuffer.Write([]float32{0.4, 0.4, 0.4, 0.4})
	m.sysBuffer.Write([]float32{0.1, 0.1, 0.1, 0.1})

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected aligned mixed chunk")
	}

	for i, sample := range mixed {
		if sample != 0.5 {
			t.Fatalf("expected sample %d to be 0.5, got %f", i, sample)
		}
	}
}

func TestMixNextChunk_EmitsWhenSystemPartialStalls(t *testing.T) {
	const chunkSize = 8
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.micBuffer.Write([]float32{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5})
	m.sysBuffer.Write([]float32{0.2, 0.2, 0.2})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected first stalled partial tick to wait")
	}

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected mixed chunk once system partial stalls")
	}

	if len(mixed) != chunkSize {
		t.Fatalf("expected chunk size %d, got %d", chunkSize, len(mixed))
	}

	for i, sample := range mixed {
		if sample != 0.5 {
			t.Fatalf("expected sample %d to be 0.5, got %f", i, sample)
		}
	}
}

func TestMixNextChunk_EmitsWhenMicPartialStalls(t *testing.T) {
	const chunkSize = 8
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.sysBuffer.Write([]float32{0.25, 0.25, 0.25, 0.25, 0.25, 0.25, 0.25, 0.25})
	m.micBuffer.Write([]float32{0.1, 0.1, 0.1})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected first stalled partial tick to wait")
	}

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected mixed chunk once microphone partial stalls")
	}

	if len(mixed) != chunkSize {
		t.Fatalf("expected chunk size %d, got %d", chunkSize, len(mixed))
	}

	for i, sample := range mixed {
		if sample != 0.25 {
			t.Fatalf("expected sample %d to be 0.25, got %f", i, sample)
		}
	}
}

func TestMixNextChunk_DropsStalledSystemPartialBeforeResumingMixed(t *testing.T) {
	const chunkSize = 4
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.micBuffer.Write([]float32{1, 1, 1, 1, 2, 2, 2, 2})
	m.sysBuffer.Write([]float32{0.25, 0.25})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected first stalled partial tick to wait")
	}

	if _, ok := m.mixNextChunk(chunkSize, state); !ok {
		t.Fatal("expected solo mic chunk after stalled system partial")
	}

	m.sysBuffer.Write([]float32{0.5, 0.5, 0.5, 0.5})

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected aligned mixed chunk after system resumes")
	}

	for i, sample := range mixed {
		if sample != 2.5 {
			t.Fatalf("expected sample %d to be 2.5, got %f", i, sample)
		}
	}
}

func TestMixNextChunk_DropsStalledMicPartialBeforeResumingMixed(t *testing.T) {
	const chunkSize = 4
	m := newMixedTestManager(chunkSize)
	state := &mixSyncState{graceTicks: 2}

	m.sysBuffer.Write([]float32{1, 1, 1, 1, 2, 2, 2, 2})
	m.micBuffer.Write([]float32{0.25, 0.25})

	if _, ok := m.mixNextChunk(chunkSize, state); ok {
		t.Fatal("expected first stalled partial tick to wait")
	}

	if _, ok := m.mixNextChunk(chunkSize, state); !ok {
		t.Fatal("expected solo system chunk after stalled microphone partial")
	}

	m.micBuffer.Write([]float32{0.5, 0.5, 0.5, 0.5})

	mixed, ok := m.mixNextChunk(chunkSize, state)
	if !ok {
		t.Fatal("expected aligned mixed chunk after microphone resumes")
	}

	for i, sample := range mixed {
		if sample != 2.5 {
			t.Fatalf("expected sample %d to be 2.5, got %f", i, sample)
		}
	}
}
