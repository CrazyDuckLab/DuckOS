//go:build !linux

package observe

import (
	"context"
	"errors"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
)

func (c *Collector) Overview(context.Context) (model.Overview, error) {
	return model.Overview{}, errors.New("Linux host observation is unavailable on this operating system")
}
