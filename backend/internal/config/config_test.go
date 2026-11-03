package config

import "testing"

func TestLoad(t *testing.T) {
	const dsn = "postgres://lexis:lexis@localhost:5433/lexis"

	tests := []struct {
		name     string
		env      map[string]string
		wantErr  bool
		wantAddr string
		wantTZ   string
	}{
		{
			name:     "defaults",
			env:      map[string]string{"LEXIS_DB_DSN": dsn},
			wantAddr: ":8080",
			wantTZ:   "Europe/Moscow",
		},
		{
			name:     "overrides",
			env:      map[string]string{"LEXIS_DB_DSN": dsn, "LEXIS_HTTP_ADDR": ":9090", "LEXIS_TZ": "Europe/Berlin"},
			wantAddr: ":9090",
			wantTZ:   "Europe/Berlin",
		},
		{
			name:    "missing dsn",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:    "bad tz",
			env:     map[string]string{"LEXIS_DB_DSN": dsn, "LEXIS_TZ": "Mars/Olympus"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// чтение из nil-map безопасно, так что лишняя проверка не нужна
			cfg, err := Load(func(k string) string { return tt.env[k] })
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got cfg %+v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.HTTPAddr != tt.wantAddr {
				t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, tt.wantAddr)
			}
			if cfg.Location.String() != tt.wantTZ {
				t.Errorf("Location = %q, want %q", cfg.Location, tt.wantTZ)
			}
			if cfg.DBDSN != dsn {
				t.Errorf("DBDSN = %q, want %q", cfg.DBDSN, dsn)
			}
		})
	}
}
