package provider

import "testing"

func TestOllamaModelCaps(t *testing.T) {
	tests := []struct {
		name        string
		wantTools   bool
		wantXML     bool
	}{
		// ── Llama ────────────────────────────────────────────────────────────
		{"llama3.1", true, true},
		{"llama3.1:8b", true, true},
		{"llama3.2:3b", true, true},
		{"llama3.3:70b-instruct", true, true},
		// llama2 is not in the table → safe default
		{"llama2", false, false},
		{"llama2:13b", false, false},

		// ── Mistral ──────────────────────────────────────────────────────────
		{"mistral", true, true},
		{"mistral:7b", true, true},
		{"mistral-nemo", true, true},
		{"mistral-nemo:12b", true, true},
		{"mistral-small:22b", true, true},
		{"mistral-large:latest", true, true},

		// ── Qwen ─────────────────────────────────────────────────────────────
		{"qwen2.5", true, true},
		{"qwen2.5:7b", true, true},
		{"qwen2.5-coder:7b-instruct", true, true},
		{"qwen3:8b", true, true},
		// qwen2 is not in the table → safe default
		{"qwen2:7b", false, false},

		// ── Gemma ────────────────────────────────────────────────────────────
		{"gemma3", true, true},
		{"gemma3:12b", true, true},
		{"gemma2", false, false},
		{"gemma2:9b", false, false},
		{"gemma", false, false},
		{"gemma:7b", false, false},

		// ── Phi ──────────────────────────────────────────────────────────────
		{"phi4", true, true},
		{"phi4:14b", true, true},
		{"phi3.5", true, true},
		{"phi3.5:3.8b", true, true},
		{"phi3", false, false},
		{"phi3:3.8b", false, false},
		// phi (bare) is not in the table → safe default
		{"phi", false, false},

		// ── DeepSeek ─────────────────────────────────────────────────────────
		{"deepseek-coder-v2", true, true},
		{"deepseek-coder-v2:16b", true, true},
		{"deepseek-r1", false, false},
		{"deepseek-r1:8b", false, false},
		{"deepseek", false, false},

		// ── Code-focused ──────────────────────────────────────────────────────
		{"codellama", false, false},
		{"codellama:7b", false, false},
		{"codellama:13b-instruct", false, false},
		{"codegemma", false, false},

		// ── Older community models ────────────────────────────────────────────
		{"vicuna", false, false},
		{"vicuna:13b", false, false},
		{"orca", false, false},
		{"solar", false, false},
		{"zephyr", false, false},
		{"openchat", false, false},

		// ── Case-insensitivity ────────────────────────────────────────────────
		{"Llama3.2:3b", true, true},
		{"MISTRAL:7B", true, true},
		{"Qwen2.5-Coder:7B-Instruct", true, true},
		{"CodeLlama:13b", false, false},

		// ── Unknown / custom models → safe default ────────────────────────────
		{"", false, false},
		{"mycompany/custom-model", false, false},
		{"unknown-model:latest", false, false},
		{"neural-chat:7b", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ollamaModelCaps(tt.name)
			if got.NativeTools != tt.wantTools {
				t.Errorf("ollamaModelCaps(%q).NativeTools = %v, want %v", tt.name, got.NativeTools, tt.wantTools)
			}
			if got.XMLFriendly != tt.wantXML {
				t.Errorf("ollamaModelCaps(%q).XMLFriendly = %v, want %v", tt.name, got.XMLFriendly, tt.wantXML)
			}
		})
	}
}
