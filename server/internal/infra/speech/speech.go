package speech

import "context"

// Recognizer 提供语音识别能力。
type Recognizer interface {
	Recognize(ctx context.Context, audio []byte, mediaType string) (string, error)
}

// Synthesizer 提供语音合成能力。
type Synthesizer interface {
	Synthesize(ctx context.Context, text string, voice string) ([]byte, error)
}
