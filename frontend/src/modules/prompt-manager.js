import { GetAllPrompts, GetPrompt, CreatePrompt, UpdatePrompt, DeletePrompt, ExecutePromptOnNote, ExecutePromptOnContent } from '../../wailsjs/go/main/App.js';

class PromptManager {
    constructor() {
        this.prompts = [];
        this.currentPrompt = null;
        this.onPromptsChanged = null;
    }

    async initialize() {
        await this.loadPrompts();
    }

    async loadPrompts() {
        try {
            this.prompts = await GetAllPrompts();
            if (this.onPromptsChanged) {
                this.onPromptsChanged(this.prompts);
            }
            return this.prompts;
        } catch (error) {
            console.error('Failed to load prompts:', error);
            throw error;
        }
    }

    async getPrompt(id) {
        try {
            this.currentPrompt = await GetPrompt(id);
            return this.currentPrompt;
        } catch (error) {
            console.error('Failed to get prompt:', error);
            throw error;
        }
    }

    async createPrompt(name, description, systemPrompt, userPrompt, temperature = 0.7, maxTokens = 512) {
        try {
            const prompt = await CreatePrompt(name, description, systemPrompt, userPrompt, temperature, maxTokens);
            await this.loadPrompts();
            return prompt;
        } catch (error) {
            console.error('Failed to create prompt:', error);
            throw error;
        }
    }

    async updatePrompt(id, name, description, systemPrompt, userPrompt, temperature, maxTokens) {
        try {
            await UpdatePrompt(id, name, description, systemPrompt, userPrompt, temperature, maxTokens);
            await this.loadPrompts();
        } catch (error) {
            console.error('Failed to update prompt:', error);
            throw error;
        }
    }

    async deletePrompt(id) {
        try {
            await DeletePrompt(id);
            await this.loadPrompts();
        } catch (error) {
            console.error('Failed to delete prompt:', error);
            throw error;
        }
    }

    async executeOnNote(promptId, noteId) {
        try {
            const result = await ExecutePromptOnNote(promptId, noteId);
            return result;
        } catch (error) {
            console.error('Failed to execute prompt on note:', error);
            throw error;
        }
    }

    async executeOnContent(promptId, content) {
        try {
            const result = await ExecutePromptOnContent(promptId, content);
            return result;
        } catch (error) {
            console.error('Failed to execute prompt on content:', error);
            throw error;
        }
    }

    getPromptById(id) {
        return this.prompts.find(p => p.id === id);
    }

    getPromptByName(name) {
        return this.prompts.find(p => p.name === name);
    }
}

export const promptManager = new PromptManager();