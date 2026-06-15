package config

import "testing"

func TestConfig_validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "정상", cfg: Config{SlackBotToken: "xoxb-1", SlackAppToken: "xapp-1"}, wantErr: false},
		{name: "bot 토큰 누락", cfg: Config{SlackAppToken: "xapp-1"}, wantErr: true},
		{name: "app 토큰 누락", cfg: Config{SlackBotToken: "xoxb-1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
