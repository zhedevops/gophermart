package logger

import (
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestInitialize(t *testing.T) {
	type args struct {
		level string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "success debug",
			args: args{
				level: "debug",
			},
			wantErr: false,
		},
		{
			name: "success panic",
			args: args{
				level: "panic",
			},
			wantErr: false,
		},
		{
			name: "failure non existing level",
			args: args{
				level: "zero",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.args.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Equal(t, fmt.Sprintf("unknown log level: %s", tt.args.level), err.Error())
			} else {
				assert.Equal(t, tt.args.level, zerolog.GlobalLevel().String())
			}
		})
	}
}
