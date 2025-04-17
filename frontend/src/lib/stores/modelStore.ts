import { writable } from 'svelte/store';
import { browser } from '$app/environment';

const MODEL_KEY = 'selected_model';

export type Model = {
  id: string;
  name: string;
  description?: string;
}

const AVAILABLE_MODELS: Model[] = [
  { 
    id: 'gemini-2.0-flash', 
    name: 'Gemini 2.0 Flash',
  },
  { 
    id: 'llama-4.0-maverick', 
    name: 'Llama 4 Maverick',
  },
  { 
    id: 'qwq-32b', 
    name: 'QwQ-32B',
  },
  { 
    id: 'gpt-4o-mini', 
    name: 'GPT-4o mini',
  },
  { 
    id: 'deepseek-r1-distill-qwen-32b', 
    name: 'DeepSeek R1 Distill Qwen 32B',
  }
];

const getInitialModel = (): string => {
  if (!browser) return AVAILABLE_MODELS[0].id;
  const stored = localStorage.getItem(MODEL_KEY);
  return stored && AVAILABLE_MODELS.some(model => model.id === stored) 
    ? stored 
    : AVAILABLE_MODELS[0].id;
};

const createModelStore = () => {
  const { subscribe, set } = writable<string>(getInitialModel());

  return {
    subscribe,
    availableModels: AVAILABLE_MODELS,
    setModel: (modelId: string) => {
      if (browser) {
        localStorage.setItem(MODEL_KEY, modelId);
      }
      set(modelId);
    },
    getModelName: (modelId: string): string => {
      return AVAILABLE_MODELS.find(model => model.id === modelId)?.name || AVAILABLE_MODELS[0].name;
    }
  };
};

export const modelStore = createModelStore(); 