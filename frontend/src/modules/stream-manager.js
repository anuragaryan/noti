import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime.js';

/**
 * StreamManager handles streaming LLM responses via Wails events
 */
class StreamManager {
    constructor() {
        this.isStreaming = false;
        this.currentText = '';
        this.onChunk = null;
        this.onComplete = null;
        this.onError = null;
        this.chunkCount = 0;
    }

    /**
     * Start listening for stream events
     * @param {Object} callbacks - Callback functions
     * @param {Function} callbacks.onChunk - Called for each chunk (chunkText, fullText, chunkIndex)
     * @param {Function} callbacks.onComplete - Called when stream completes (finalText, chunk)
     * @param {Function} callbacks.onError - Called on error (errorMessage)
     */
    startListening(callbacks) {
        // Clean up any existing listeners
        this.cleanup();

        this.onChunk = callbacks.onChunk;
        this.onComplete = callbacks.onComplete;
        this.onError = callbacks.onError;
        this.currentText = '';
        this.chunkCount = 0;
        this.isStreaming = true;

        // Listen for chunk events
        EventsOn('llm:stream:chunk', (chunk) => {
            if (!this.isStreaming) return;
            
            this.currentText += chunk.text;
            this.chunkCount++;
            
            if (this.onChunk) {
                this.onChunk(chunk.text, this.currentText, chunk.index);
            }
        });

        // Listen for completion events
        EventsOn('llm:stream:done', (chunk) => {
            this.isStreaming = false;
            
            if (this.onComplete) {
                this.onComplete(this.currentText, chunk);
            }
            
            this.cleanup();
        });

        // Listen for error events
        EventsOn('llm:stream:error', (error) => {
            this.isStreaming = false;
            
            if (this.onError) {
                this.onError(error);
            }
            
            this.cleanup();
        });

        console.log('[StreamManager] Started listening for stream events');
    }

    /**
     * Stop listening and cleanup
     */
    cleanup() {
        EventsOff('llm:stream:chunk');
        EventsOff('llm:stream:done');
        EventsOff('llm:stream:error');
        this.isStreaming = false;
        console.log('[StreamManager] Cleaned up event listeners');
    }

    /**
     * Cancel the current stream
     */
    cancel() {
        if (this.isStreaming) {
            console.log('[StreamManager] Cancelling stream');
            this.cleanup();
        }
    }

    /**
     * Get current accumulated text
     * @returns {string} The accumulated text from all chunks
     */
    getCurrentText() {
        return this.currentText;
    }

    /**
     * Check if currently streaming
     * @returns {boolean} True if streaming is in progress
     */
    getIsStreaming() {
        return this.isStreaming;
    }

    /**
     * Get the number of chunks received
     * @returns {number} The chunk count
     */
    getChunkCount() {
        return this.chunkCount;
    }
}

// Export singleton instance
export const streamManager = new StreamManager();