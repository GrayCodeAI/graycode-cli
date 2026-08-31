module github.com/GrayCodeAI/hawk

go 1.26.6

// The charmbracelet v2 modules (bubbles, bubbletea, lipgloss, glamour, huh) have
// moved their module paths from github.com/charmbracelet/... to charm.land/...
// but their Go import paths still use the old github.com/charmbracelet/... paths.
// The import paths in the Go source have been migrated to charm.land/... to match.

require (
	charm.land/bubbles/v2 v2.1.1
	charm.land/bubbletea/v2 v2.0.8
	charm.land/lipgloss/v2 v2.0.5
	github.com/GrayCodeAI/eagle v0.0.0-20260831121050-12ea8cc11b16
	github.com/GrayCodeAI/eyrie v0.0.0-20260831133556-13f65882f1c9
	github.com/GrayCodeAI/harrier v0.0.0-20260831121051-a93a92eeec6d
	github.com/GrayCodeAI/kestrel v0.0.0-20260831121051-606ec6d9b867
	github.com/GrayCodeAI/merlin v0.0.0-20260831133238-f31093d609bc
	github.com/GrayCodeAI/shrike v0.0.0-20260831133117-d530178858b4
	github.com/alecthomas/chroma/v2 v2.26.1
	github.com/bwmarrin/discordgo v0.28.1
	github.com/charmbracelet/x/ansi v0.11.7
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f
	github.com/chromedp/chromedp v0.16.0
	github.com/creack/pty v1.1.24
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68
	github.com/gofrs/flock v0.13.0
	github.com/google/uuid v1.6.0
	github.com/mattn/go-runewidth v0.0.27
	github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/tetratelabs/wazero v1.12.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.20.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0
	go.opentelemetry.io/otel/log v0.20.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/log v0.20.0
	go.opentelemetry.io/otel/sdk/metric v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
	golang.org/x/image v0.45.0
	golang.org/x/mod v0.38.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	golang.org/x/text v0.41.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.51.0
)

// Reachable main-branch revisions replace unpublished historical pins carried
// by the current engine module commits. Remove these excludes after the next
// ordered Eagle -> Falcon -> engine release.
exclude (
	github.com/GrayCodeAI/eagle v0.1.13
	github.com/GrayCodeAI/falcon v0.1.4
	github.com/GrayCodeAI/falcon v0.1.5
	github.com/GrayCodeAI/falcon v0.1.6-0.20260825010843-82c1c610efe3
)

require (
	github.com/GrayCodeAI/falcon v0.0.0-20260831121050-870f4262da2b // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/denisbrodbeck/machineid v1.0.1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-faster/errors v0.8.0 // indirect
	github.com/go-faster/jx v1.2.0 // indirect
	github.com/go-faster/yaml v0.4.6 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ogen-go/ogen v1.23.0 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

require (
	cel.dev/expr v0.25.2 // indirect
	charm.land/glamour/v2 v2.0.1 // indirect
	charm.land/huh/v2 v2.0.3 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/BobuSumisu/aho-corasick v1.0.3 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/STARRY-S/zip v0.2.3 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/betterleaks/betterleaks v1.5.0 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/sevenzip v1.6.4 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/catppuccin/go v0.3.0 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260703014108-f5a850f9c2b7 // indirect
	github.com/charmbracelet/x/exp/ordered v0.1.0 // indirect
	github.com/charmbracelet/x/exp/slice v0.0.0-20260608090822-c3ad58c6c9e5 // indirect
	github.com/charmbracelet/x/exp/strings v0.1.0 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/dsnet/compress v0.0.2-0.20230904184137-39efe44ab707 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/entireio/auth-go v0.5.2 // indirect
	github.com/fatih/semgroup v1.3.0 // indirect
	github.com/gitleaks/go-gitdiff v0.9.1 // indirect
	github.com/go-git/gcfg/v2 v2.0.2 // indirect
	github.com/go-git/go-billy/v6 v6.0.0-alpha.2 // indirect
	github.com/go-git/go-git/v6 v6.0.0-alpha.5 // indirect; indirect — TODO: alpha, API-unstable; upgrade when v6 reaches stable or pin back to go-git/v5
	github.com/go-git/x/plugin/objectsigner/auto v0.1.1-0.20260624122410-382b2905c041 // indirect
	github.com/go-git/x/plugin/objectsigner/gpg v0.2.1-0.20260624122410-382b2905c041 // indirect
	github.com/go-git/x/plugin/objectsigner/program v0.0.0-20260624122410-382b2905c041 // indirect
	github.com/go-git/x/plugin/objectsigner/ssh v0.2.1-0.20260624122410-382b2905c041 // indirect
	github.com/go-sprout/sprout v1.0.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/cel-go v0.28.1 // indirect
	github.com/google/go-github/v72 v72.0.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hiddeco/sshsig v0.2.0 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/compress v1.19.1
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mholt/archives v0.1.6-0.20260429171216-ef71b7a32fae // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/mikelolasagasti/xz v1.0.1 // indirect
	github.com/minio/minlz v1.1.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/nwaples/rardecode/v2 v2.2.3 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/pkoukk/tiktoken-go v0.1.8 // indirect
	github.com/posthog/posthog-go v1.22.0 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20260209031235-2402fdf4a9ed // indirect
	github.com/shurcooL/graphql v0.0.0-20240915155400-7ee5256398cf // indirect
	github.com/sorairolake/lzip-go v0.3.8 // indirect
	github.com/stangelandcl/ppmd v0.1.1 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/yuin/goldmark v1.8.2 // indirect
	github.com/yuin/goldmark-emoji v1.0.6 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	go4.org v0.0.0-20260112195520-a5071408f32f // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/time v0.15.0
)

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/GrayCodeAI/swift v0.0.0-20260831121051-6579cc97156a
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tiktoken-go/tokenizer v0.8.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20260603202125-055de637280b // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.82.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	modernc.org/libc v1.72.5 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
