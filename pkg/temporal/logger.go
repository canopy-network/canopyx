package temporal

import "go.uber.org/zap"

// ZapAdapter is a Temporal logger adapter for Zap.
type ZapAdapter struct{ *zap.SugaredLogger }

// NewZapAdapter creates a new Temporal logger adapter from a Zap logger.
func NewZapAdapter(logger *zap.Logger) *ZapAdapter {
	// Temporal adapter is Sugared since we need to pass forward the keyvals
	return &ZapAdapter{logger.Sugar()}
}

func (z *ZapAdapter) Debug(msg string, keyvals ...interface{}) {
	z.Debugw(msg, keyvals...)
}
func (z *ZapAdapter) Info(msg string, keyvals ...interface{}) { z.Infow(msg, keyvals...) }
func (z *ZapAdapter) Warn(msg string, keyvals ...interface{}) { z.Warnw(msg, keyvals...) }
func (z *ZapAdapter) Error(msg string, keyvals ...interface{}) {
	z.Errorw(msg, keyvals...)
}
