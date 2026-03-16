export namespace domain {
	
	export class AudioDevice {
	    id: string;
	    name: string;
	    source: number;
	    isDefault: boolean;
	    sampleRate: number;
	    channels: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.isDefault = source["isDefault"];
	        this.sampleRate = source["sampleRate"];
	        this.channels = source["channels"];
	    }
	}
	export class AudioMixerConfig {
	    microphoneGain: number;
	    systemGain: number;
	    mixMode: string;
	
	    static createFrom(source: any = {}) {
	        return new AudioMixerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.microphoneGain = source["microphoneGain"];
	        this.systemGain = source["systemGain"];
	        this.mixMode = source["mixMode"];
	    }
	}
	export class AudioSettings {
	    defaultSource: string;
	    mixer: AudioMixerConfig;
	
	    static createFrom(source: any = {}) {
	        return new AudioSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultSource = source["defaultSource"];
	        this.mixer = this.convertValues(source["mixer"], AudioMixerConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LLMConfig {
	    provider: string;
	    modelName: string;
	    apiEndpoint: string;
	    apiKey: string;
	    temperature: number;
	    maxTokens: number;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.modelName = source["modelName"];
	        this.apiEndpoint = source["apiEndpoint"];
	        this.apiKey = source["apiKey"];
	        this.temperature = source["temperature"];
	        this.maxTokens = source["maxTokens"];
	    }
	}
	export class Config {
	    modelName: string;
	    sttLanguage: string;
	    llm: LLMConfig;
	    audio: AudioSettings;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modelName = source["modelName"];
	        this.sttLanguage = source["sttLanguage"];
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.audio = this.convertValues(source["audio"], AudioSettings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Folder {
	    id: string;
	    name: string;
	    nameOnDisk: string;
	    parentId: string;
	    // Go type: time
	    createdAt: any;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.nameOnDisk = source["nameOnDisk"];
	        this.parentId = source["parentId"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.order = source["order"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LLMMessage {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class LLMResponse {
	    text: string;
	    tokensUsed: number;
	    model: string;
	    finishReason: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.tokensUsed = source["tokensUsed"];
	        this.model = source["model"];
	        this.finishReason = source["finishReason"];
	    }
	}
	export class ModelOption {
	    id: number;
	    code: string;
	    name: string;
	    isRecommended: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.code = source["code"];
	        this.name = source["name"];
	        this.isRecommended = source["isRecommended"];
	        this.note = source["note"];
	    }
	}
	export class Note {
	    id: string;
	    title: string;
	    fileStem: string;
	    folderId: string;
	    transcriptActivated: boolean;
	    markdownContent: string;
	    transcriptContent: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new Note(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.fileStem = source["fileStem"];
	        this.folderId = source["folderId"];
	        this.transcriptActivated = source["transcriptActivated"];
	        this.markdownContent = source["markdownContent"];
	        this.transcriptContent = source["transcriptContent"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.order = source["order"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Prompt {
	    id: string;
	    name: string;
	    description: string;
	    systemPrompt: string;
	    userPrompt: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Prompt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.systemPrompt = source["systemPrompt"];
	        this.userPrompt = source["userPrompt"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PromptExecutionResult {
	    promptName: string;
	    input: string;
	    output: string;
	    tokensUsed: number;
	    // Go type: time
	    executedAt: any;
	    llmResponse?: LLMResponse;
	
	    static createFrom(source: any = {}) {
	        return new PromptExecutionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.promptName = source["promptName"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.tokensUsed = source["tokensUsed"];
	        this.executedAt = this.convertValues(source["executedAt"], null);
	        this.llmResponse = this.convertValues(source["llmResponse"], LLMResponse);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SearchMatch {
	    note: Note;
	    line: number;
	    snippet: string;
	    sourceLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchMatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.note = this.convertValues(source["note"], Note);
	        this.line = source["line"];
	        this.snippet = source["snippet"];
	        this.sourceLabel = source["sourceLabel"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TranscriptionResult {
	    text: string;
	    language: string;
	    duration: number;
	    timestamp: string;
	    isPartial: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TranscriptionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.language = source["language"];
	        this.duration = source["duration"];
	        this.timestamp = source["timestamp"];
	        this.isPartial = source["isPartial"];
	    }
	}

}

