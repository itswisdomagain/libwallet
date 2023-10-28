package walletdata

import (
	"github.com/asdine/storm"
	"github.com/decred/slog"
)

type UserConfigReader interface {
	ReadUserConfigValue(key string, valueOut interface{}) error
}

func ReadUserConfigValue[T any](db UserConfigReader, key string, defaultValue ...T) T {
	var v T
	if err := db.ReadUserConfigValue(key, &v); err != nil {
		tryLogError("ReadUserConfigValue", db, err)
		if len(defaultValue) > 0 {
			v = defaultValue[0]
		}
	}
	return v
}

func tryLogError(fn string, maybeLogger interface{}, err error) {
	if err == nil || err == storm.ErrNotFound {
		return
	}
	if log, ok := maybeLogger.(slog.Logger); ok {
		log.Errorf("%s error: %v", fn, err)
	}
}
