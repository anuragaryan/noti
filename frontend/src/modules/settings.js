import { GetConfig, SaveConfig } from '../../wailsjs/go/main/App';
import DOMRefs from '../dom-refs.js';

class Settings {
    constructor() {
        this.modal = null;
        this.currentConfig = null;
        this.isInitialized = false;
    }

    initialize() {
        if (this.isInitialized) return;

        this.modal = document.getElementById('settingsModal');
        this.setupEventListeners();
        this.isInitialized = true;
    }

    setupEventListeners() {
        // Close button
        const closeBtn = document.getElementById('closeSettingsBtn');
        if (closeBtn) {
            closeBtn.addEventListener('click', () => this.close());
        }

        // Cancel button
        const cancelBtn = document.getElementById('cancelSettingsBtn');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', () => this.close());
        }

        // Save button
        const saveBtn = document.getElementById('saveSettingsBtn');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => this.save());
        }

        // API key visibility toggle
        const toggleApiKeyBtn = document.getElementById('toggleApiKeyBtn');
        const apiKeyInput = document.getElementById('llmApiKey');
        if (toggleApiKeyBtn && apiKeyInput) {
            toggleApiKeyBtn.addEventListener('click', () => {
                const type = apiKeyInput.type === 'password' ? 'text' : 'password';
                apiKeyInput.type = type;
                toggleApiKeyBtn.textContent = type === 'password' ? '👁' : '🙈';
            });
        }

        // Temperature slider
        const tempSlider = document.getElementById('llmTemperature');
        const tempValue = document.getElementById('llmTempValue');
        if (tempSlider && tempValue) {
            tempSlider.addEventListener('input', (e) => {
                tempValue.textContent = parseFloat(e.target.value).toFixed(1);
            });
        }

        // Microphone gain slider
        const micGainSlider = document.getElementById('audioMicGain');
        const micGainValue = document.getElementById('audioMicGainValue');
        if (micGainSlider && micGainValue) {
            micGainSlider.addEventListener('input', (e) => {
                micGainValue.textContent = parseFloat(e.target.value).toFixed(1);
            });
        }

        // System gain slider
        const sysGainSlider = document.getElementById('audioSystemGain');
        const sysGainValue = document.getElementById('audioSystemGainValue');
        if (sysGainSlider && sysGainValue) {
            sysGainSlider.addEventListener('input', (e) => {
                sysGainValue.textContent = parseFloat(e.target.value).toFixed(1);
            });
        }

        // Provider change handler
        const providerSelect = document.getElementById('llmProvider');
        if (providerSelect) {
            providerSelect.addEventListener('change', (e) => {
                this.updateProviderFields(e.target.value);
            });
        }

        // Close modal on background click
        if (this.modal) {
            this.modal.addEventListener('click', (e) => {
                if (e.target === this.modal) {
                    this.close();
                }
            });
        }
    }

    updateProviderFields(provider) {
        const apiEndpointGroup = document.getElementById('llmApiEndpointGroup');
        const apiKeyGroup = document.getElementById('llmApiKeyGroup');

        if (provider === 'local') {
            // For local provider, hide API key, show endpoint
            if (apiKeyGroup) apiKeyGroup.style.display = 'none';
            if (apiEndpointGroup) {
                apiEndpointGroup.style.display = 'block';
                const label = apiEndpointGroup.querySelector('label');
                if (label) label.textContent = 'Local Server Endpoint';
                const hint = apiEndpointGroup.querySelector('.form-hint');
                if (hint) hint.textContent = 'URL of your local Llama.cpp server (e.g., http://localhost:8080)';
            }
        } else {
            // For API provider, show both
            if (apiKeyGroup) apiKeyGroup.style.display = 'block';
            if (apiEndpointGroup) {
                apiEndpointGroup.style.display = 'block';
                const label = apiEndpointGroup.querySelector('label');
                if (label) label.textContent = 'API Endpoint (optional)';
                const hint = apiEndpointGroup.querySelector('.form-hint');
                if (hint) hint.textContent = 'Leave empty to use the default endpoint for your provider';
            }
        }
    }

    async open() {
        try {
            // Load current configuration
            this.currentConfig = await GetConfig();
            this.populateForm(this.currentConfig);
            
            // Show modal
            if (this.modal) {
                this.modal.style.display = 'flex';
            }
        } catch (error) {
            console.error('Failed to load configuration:', error);
            alert('Failed to load settings. Please try again.');
        }
    }

    populateForm(config) {
        // STT Settings
        const sttChunkSeconds = document.getElementById('sttChunkSeconds');
        if (sttChunkSeconds) {
            sttChunkSeconds.value = config.realtimeTranscriptionChunkSeconds || 3;
        }

        const sttModelName = document.getElementById('sttModelName');
        if (sttModelName) {
            sttModelName.value = config.modelName || 'base.en';
        }

        // LLM Settings
        const llmProvider = document.getElementById('llmProvider');
        if (llmProvider) {
            llmProvider.value = config.llm?.provider || 'api';
            this.updateProviderFields(llmProvider.value);
        }

        const llmModelName = document.getElementById('llmModelName');
        if (llmModelName) {
            llmModelName.value = config.llm?.modelName || '';
        }

        const llmApiEndpoint = document.getElementById('llmApiEndpoint');
        if (llmApiEndpoint) {
            llmApiEndpoint.value = config.llm?.apiEndpoint || '';
        }

        const llmApiKey = document.getElementById('llmApiKey');
        if (llmApiKey) {
            llmApiKey.value = config.llm?.apiKey || '';
        }

        const llmTemperature = document.getElementById('llmTemperature');
        const llmTempValue = document.getElementById('llmTempValue');
        if (llmTemperature && llmTempValue) {
            const temp = config.llm?.temperature || 0.7;
            llmTemperature.value = temp;
            llmTempValue.textContent = temp.toFixed(1);
        }

        const llmMaxTokens = document.getElementById('llmMaxTokens');
        if (llmMaxTokens) {
            llmMaxTokens.value = config.llm?.maxTokens || 2048;
        }

        // Audio Settings
        const audioDefaultSource = document.getElementById('audioDefaultSource');
        if (audioDefaultSource) {
            audioDefaultSource.value = config.audio?.defaultSource || 'microphone';
        }

        const audioSampleRate = document.getElementById('audioSampleRate');
        if (audioSampleRate) {
            audioSampleRate.value = config.audio?.sampleRate || 16000;
        }

        const audioMicGain = document.getElementById('audioMicGain');
        const audioMicGainValue = document.getElementById('audioMicGainValue');
        if (audioMicGain && audioMicGainValue) {
            const gain = config.audio?.mixer?.microphoneGain || 1.0;
            audioMicGain.value = gain;
            audioMicGainValue.textContent = gain.toFixed(1);
        }

        const audioSystemGain = document.getElementById('audioSystemGain');
        const audioSystemGainValue = document.getElementById('audioSystemGainValue');
        if (audioSystemGain && audioSystemGainValue) {
            const gain = config.audio?.mixer?.systemGain || 1.0;
            audioSystemGain.value = gain;
            audioSystemGainValue.textContent = gain.toFixed(1);
        }

        const audioMixMode = document.getElementById('audioMixMode');
        if (audioMixMode) {
            audioMixMode.value = config.audio?.mixer?.mixMode || 'sum';
        }
    }

    collectFormData() {
        const getIntValue = (id, defaultValue) => {
            const value = document.getElementById(id)?.value;
            const parsed = parseInt(value);
            return isNaN(parsed) ? defaultValue : parsed;
        };

        const getFloatValue = (id, defaultValue) => {
            const value = document.getElementById(id)?.value;
            const parsed = parseFloat(value);
            return isNaN(parsed) ? defaultValue : parsed;
        };

        const config = {
            realtimeTranscriptionChunkSeconds: getIntValue('sttChunkSeconds', 3),
            modelName: document.getElementById('sttModelName')?.value || 'base.en',
            llm: {
                provider: document.getElementById('llmProvider')?.value || 'api',
                modelName: document.getElementById('llmModelName')?.value || '',
                apiEndpoint: document.getElementById('llmApiEndpoint')?.value || '',
                apiKey: document.getElementById('llmApiKey')?.value || '',
                temperature: getFloatValue('llmTemperature', 0.7),
                maxTokens: getIntValue('llmMaxTokens', 2048)
            },
            audio: {
                defaultSource: document.getElementById('audioDefaultSource')?.value || 'microphone',
                sampleRate: getIntValue('audioSampleRate', 16000),
                mixer: {
                    microphoneGain: getFloatValue('audioMicGain', 1.0),
                    systemGain: getFloatValue('audioSystemGain', 1.0),
                    mixMode: document.getElementById('audioMixMode')?.value || 'sum'
                }
            }
        };

        return config;
    }

    validateConfig(config) {
        const errors = [];
        
        // Clear previous error states
        this.clearValidationErrors();

        // Validate STT settings
        if (config.realtimeTranscriptionChunkSeconds < 1 || config.realtimeTranscriptionChunkSeconds > 30) {
            errors.push({ field: 'sttChunkSeconds', message: 'Transcription chunk duration must be between 1 and 30 seconds' });
        }

        if (!config.modelName || config.modelName.trim() === '') {
            errors.push({ field: 'sttModelName', message: 'STT model name is required' });
        }

        // Validate LLM settings
        if (config.llm.temperature < 0 || config.llm.temperature > 2) {
            errors.push({ field: 'llmTemperature', message: 'LLM temperature must be between 0 and 2' });
        }

        if (config.llm.maxTokens < 50 || config.llm.maxTokens > 8000) {
            errors.push({ field: 'llmMaxTokens', message: 'LLM max tokens must be between 50 and 8000' });
        }

        if (config.llm.provider === 'api' && (!config.llm.apiKey || config.llm.apiKey.trim() === '')) {
            errors.push({ field: 'llmApiKey', message: 'API key is required when using API provider' });
        }

        // Validate audio settings
        const validSampleRates = [8000, 16000, 22050, 44100, 48000];
        if (!validSampleRates.includes(config.audio.sampleRate)) {
            errors.push({ field: 'audioSampleRate', message: 'Invalid sample rate' });
        }

        if (config.audio.mixer.microphoneGain < 0 || config.audio.mixer.microphoneGain > 2) {
            errors.push({ field: 'audioMicGain', message: 'Microphone gain must be between 0 and 2' });
        }

        if (config.audio.mixer.systemGain < 0 || config.audio.mixer.systemGain > 2) {
            errors.push({ field: 'audioSystemGain', message: 'System gain must be between 0 and 2' });
        }

        return errors;
    }

    clearValidationErrors() {
        // Remove all error classes and messages
        const errorInputs = document.querySelectorAll('.modal-input.error, .range-input.error');
        errorInputs.forEach(input => {
            input.classList.remove('error');
        });
        
        const errorMessages = document.querySelectorAll('.error-message');
        errorMessages.forEach(msg => msg.remove());
    }

    showValidationErrors(errors) {
        errors.forEach(error => {
            const field = document.getElementById(error.field);
            if (field) {
                // Add error class to input
                field.classList.add('error');
                
                // Create and insert error message
                const errorMsg = document.createElement('small');
                errorMsg.className = 'error-message';
                errorMsg.style.color = '#e74c3c';
                errorMsg.style.display = 'block';
                errorMsg.style.marginTop = '4px';
                errorMsg.textContent = error.message;
                
                // Insert after the field or its parent form-group
                const formGroup = field.closest('.form-group');
                if (formGroup) {
                    formGroup.appendChild(errorMsg);
                } else {
                    field.parentNode.insertBefore(errorMsg, field.nextSibling);
                }
                
                // Scroll to first error
                if (errors.indexOf(error) === 0) {
                    field.scrollIntoView({ behavior: 'smooth', block: 'center' });
                }
            }
        });
    }

    async save() {
        try {
            // Collect form data
            const config = this.collectFormData();

            // Validate
            const errors = this.validateConfig(config);
            if (errors.length > 0) {
                // Show errors on the fields
                this.showValidationErrors(errors);
                
                // Also show a summary alert
                const errorMessages = errors.map(e => e.message).join('\n');
                alert('Please fix the following errors:\n\n' + errorMessages);
                return;
            }

            // Save configuration
            const saveBtn = document.getElementById('saveSettingsBtn');
            if (saveBtn) {
                saveBtn.disabled = true;
                saveBtn.textContent = 'Saving...';
            }

            await SaveConfig(config);

            // Success
            alert('Settings saved successfully! Some changes may require restarting the application.');
            this.close();

        } catch (error) {
            console.error('Failed to save configuration:', error);
            alert('Failed to save settings: ' + error.message);
        } finally {
            const saveBtn = document.getElementById('saveSettingsBtn');
            if (saveBtn) {
                saveBtn.disabled = false;
                saveBtn.textContent = 'Save Settings';
            }
        }
    }

    close() {
        if (this.modal) {
            this.modal.style.display = 'none';
        }
        
        // Reset API key visibility
        const apiKeyInput = document.getElementById('llmApiKey');
        const toggleBtn = document.getElementById('toggleApiKeyBtn');
        if (apiKeyInput && toggleBtn) {
            apiKeyInput.type = 'password';
            toggleBtn.textContent = '👁';
        }
    }
}

// Create singleton instance
const settings = new Settings();

export default settings;