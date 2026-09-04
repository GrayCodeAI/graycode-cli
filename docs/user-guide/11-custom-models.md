# Custom Models

Graycode connects to custom model endpoints through GraycodeRouter for alternative providers, self-hosted models, and overriding built-in settings.

---

## Supported Providers

Graycode works with any LLM provider. Built-in support includes:

| Provider | ID | Key |
|----------|-----|-----|
| xAI Grok | `grok` | `XAI_API_KEY` |
| Anthropic Claude | `anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Google Gemini | `gemini` | `GEMINI_API_KEY` |
| DeepSeek | `deepseek` | `DEEPSEEK_API_KEY` |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` |

---

## Selecting a Model

### CLI Flag

```bash
graycode -m gpt-4o -p "Hello"
```

### Slash Command

In the TUI:

```
/model gpt-4o
/model claude-3-5-sonnet
```

### Config Default

```json
// ~/.graycode/settings.json
{
  "default_provider": "openai",
  "default_model": "gpt-4o"
}
```

---

## Configuring Custom Models

Add custom models in `~/.graycode/settings.json`:

```json
{
  "models": {
    "my-local-model": {
      "model": "codellama",
      "base_url": "http://localhost:11434/v1",
      "name": "CodeLlama (Local)",
      "context_window": 128000
    },
    "custom-openai": {
      "model": "gpt-4-turbo",
      "base_url": "https://api.example.com/v1",
      "api_key": "sk-...",
      "name": "Custom GPT-4"
    }
  }
}
```

### Credential Resolution

Graycode resolves credentials in this order:

1. Per-model `api_key` field
2. Environment variable (`env_key`)
3. Signed-in session token
4. Global fallback (`XAI_API_KEY`, etc.)

---

## Provider Examples

### Anthropic (Claude)

```json
{
  "models": {
    "claude-opus": {
      "model": "claude-opus-4-6",
      "base_url": "https://api.anthropic.com/v1",
      "name": "Claude Opus",
      "extra_headers": {
        "x-api-key": "sk-ant-...",
        "anthropic-version": "2023-06-01"
      }
    }
  }
}
```

### Ollama (Local Models)

```json
{
  "models": {
    "ollama-llama": {
      "model": "llama-3.1-70b",
      "base_url": "http://localhost:11434/v1",
      "name": "Llama 3.1 (Local)"
    }
  }
}
```

Make sure Ollama is running:

```bash
ollama serve
ollama pull llama-3.1-70b
```

---

## Deployment-Aware Routing

Enable deployment-aware routing to use GraycodeRouter's model catalog:

```bash
export GRAYCODE_DEPLOYMENT_ROUTING=true
graycode
```

Or in settings:

```json
{
  "deployment_routing": true
}
```

Refresh the catalog:

```
/refresh-model-catalog
```

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Project Rules](12-project-rules.md) | AGENTS.md configuration |
| [Memory](13-memory.md) | Cross-session memory via harrier |

---

© 2026 GrayCode AI. All rights reserved.