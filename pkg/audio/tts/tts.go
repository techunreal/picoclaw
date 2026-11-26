package tts

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/audio/tts/local"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type TTSProvider interface {
	Name() string
	Synthesize(ctx context.Context, text string) (io.ReadCloser, error)
}

type ttsAudioMetaProvider interface {
	AudioFileMeta() (fileExt string, contentType string)
}

// localTTSProvider lazily starts the PicoClaw-managed sidecar for an installed
// local model, then delegates to its OpenAI-compatible provider.
type localTTSProvider struct {
	modelID string
	inner   TTSProvider
}

func (p *localTTSProvider) Name() string { return p.inner.Name() }

func (p *localTTSProvider) Synthesize(ctx context.Context, text string) (io.ReadCloser, error) {
	if err := local.Default().EnsureRunning(p.modelID); err != nil {
		return nil, fmt.Errorf("local TTS model is not running: %w", err)
	}
	return p.inner.Synthesize(ctx, text)
}

func providerFromModelConfig(mc *config.ModelConfig, voice string) TTSProvider {
	if mc == nil || mc.APIKey() == "" {
		return nil
	}

	protocol, modelID := providers.ExtractProtocol(mc)
	if modelID == "" {
		modelID = strings.TrimSpace(mc.Model)
	}

	switch protocol {
	case "mimo":
		return NewMimoTTSProvider(mc.APIKey(), providers.ResolveAPIBase(mc), modelID, mc.Proxy)
	default:
		options := openAITTSOptionsFromModelConfig(mc)
		if strings.TrimSpace(voice) != "" {
			options.Voice = strings.TrimSpace(voice)
		}
		provider := NewOpenAITTSProviderWithOptions(
			mc.APIKey(),
			providers.ResolveAPIBase(mc),
			mc.Proxy,
			modelID,
			options,
		)
		if localID, ok := mc.ExtraBody["picoclaw_local_tts"].(string); ok &&
			strings.TrimSpace(localID) != "" {
			return &localTTSProvider{modelID: strings.TrimSpace(localID), inner: provider}
		}
		return provider
	}
}

func openAITTSOptionsFromModelConfig(mc *config.ModelConfig) OpenAITTSOptions {
	options := OpenAITTSOptions{}
	if mc == nil || mc.ExtraBody == nil {
		return options
	}

	if voice, ok := mc.ExtraBody["voice"].(string); ok {
		options.Voice = strings.TrimSpace(voice)
	}
	if responseFormat, ok := mc.ExtraBody["response_format"].(string); ok {
		options.ResponseFormat = strings.TrimSpace(responseFormat)
	}
	return options
}

func DetectTTS(cfg *config.Config) TTSProvider {
	if cfg == nil {
		return nil
	}

	if modelName := strings.TrimSpace(cfg.Voice.TTSModelName); modelName != "" {
		if provider := ProviderForModel(cfg, modelName, ""); provider != nil {
			return provider
		}
	}

	for _, mc := range cfg.ModelList {
		if strings.Contains(strings.ToLower(mc.Model), "tts") && mc.APIKey() != "" {
			if provider := providerFromModelConfig(mc, cfg.Voice.TTSVoices[mc.ModelName]); provider != nil {
				return provider
			}
		}
	}
	return nil
}

// ProviderForModel builds a TTS provider for one model entry without changing
// the saved voice configuration. An explicit voice overrides the per-model
// stored voice, which lets the UI test unsaved selections.
func ProviderForModel(cfg *config.Config, modelName, voice string) TTSProvider {
	if cfg == nil {
		return nil
	}
	mc, err := cfg.GetModelConfig(strings.TrimSpace(modelName))
	if err != nil || mc == nil {
		return nil
	}
	selectedVoice := strings.TrimSpace(voice)
	if selectedVoice == "" {
		selectedVoice = cfg.Voice.TTSVoices[mc.ModelName]
	}
	return providerFromModelConfig(mc, selectedVoice)
}

// DetectTTSForLanguage chooses a local model matching the requested language.
// It returns nil when the caller asked for a language that no installed model
// supports; read-aloud then falls back to the browser instead of reading with
// the wrong language model.
func DetectTTSForLanguage(cfg *config.Config, language string) TTSProvider {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return DetectTTS(cfg)
	}
	if cfg == nil {
		return nil
	}
	if mapped := strings.TrimSpace(cfg.Voice.TTSLanguageModels[language]); mapped != "" {
		if provider := ProviderForModel(cfg, mapped, ""); provider != nil {
			return provider
		}
	}
	if selected := strings.TrimSpace(cfg.Voice.TTSModelName); selected != "" {
		if installedLanguage, isLocal := localModelLanguage(cfg, selected); !isLocal || installedLanguage == "" || installedLanguage == language {
			return ProviderForModel(cfg, selected, "")
		}
	}
	for _, mc := range cfg.ModelList {
		id, isLocal := localModelID(mc)
		if !isLocal {
			continue
		}
		installed, ok := local.Default().InstalledByID(id)
		if !ok || installed.Language != language {
			continue
		}
		if provider := ProviderForModel(cfg, mc.ModelName, ""); provider != nil {
			return provider
		}
	}
	return nil
}

func localModelID(mc *config.ModelConfig) (string, bool) {
	if mc == nil {
		return "", false
	}
	id, ok := mc.ExtraBody["picoclaw_local_tts"].(string)
	id = strings.TrimSpace(id)
	return id, ok && id != ""
}

func localModelLanguage(cfg *config.Config, modelName string) (string, bool) {
	mc, err := cfg.GetModelConfig(modelName)
	if err != nil || mc == nil {
		return "", false
	}
	id, isLocal := localModelID(mc)
	if !isLocal {
		return "", false
	}
	installed, ok := local.Default().InstalledByID(id)
	if !ok {
		return "", true
	}
	return strings.ToLower(strings.TrimSpace(installed.Language)), true
}

// SynthesizeAndStore synthesizes text to speech and registers it in the media store, returning the media reference.
func SynthesizeAndStore(
	ctx context.Context,
	provider TTSProvider,
	store media.MediaStore,
	text string,
	filename string,
	channel string,
	chatID string,
) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("tts provider is not configured")
	}
	if store == nil {
		return "", fmt.Errorf("media store not configured")
	}
	if channel == "" || chatID == "" {
		return "", fmt.Errorf("no target channel/chat available")
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("text is required")
	}

	stream, err := provider.Synthesize(ctx, text)
	if err != nil {
		return "", fmt.Errorf("tts synthesize failed: %w", err)
	}
	defer stream.Close()

	err = os.MkdirAll(media.TempDir(), 0o700)
	if err != nil {
		return "", fmt.Errorf("failed to create media temp dir: %w", err)
	}

	fileExt := ".ogg"
	contentType := "audio/ogg"
	if provider.Name() == "mimo-tts" {
		fileExt = ".mp3"
		contentType = "audio/mpeg"
	}
	if metaProvider, ok := stream.(ttsAudioMetaProvider); ok {
		if ext, ct := metaProvider.AudioFileMeta(); ext != "" && ct != "" {
			fileExt = ext
			contentType = ct
		}
	}

	file, err := os.CreateTemp(media.TempDir(), "tts-*"+fileExt)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(file.Name())
		}
	}()

	_, err = io.Copy(file, stream)
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("failed to write tts audio: %w", err)
	}

	err = file.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close tts audio file: %w", err)
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = fmt.Sprintf("tts-%d%s", time.Now().Unix(), fileExt)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		filename += fileExt
	} else if ext != fileExt {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + fileExt
	}

	scope := fmt.Sprintf("tool:send_tts:%s:%s:%d", channel, chatID, time.Now().UnixNano())
	ref, err := store.Store(file.Name(), media.MediaMeta{
		Filename:    filename,
		ContentType: contentType,
		Source:      "tool:send_tts",
	}, scope)
	if err != nil {
		return "", fmt.Errorf("failed to register audio: %w", err)
	}
	removeTemp = false

	return ref, nil
}
