// Package icons is hawk's centralized icon registry.
//
// Hawk never emits emoji or one-off Unicode glyphs in its CLI/TUI output.
// Every glyph hawk renders goes through this package. Each glyph has two
// forms:
//
//   - A Nerd Font PUA (Private Use Area) codepoint (U+E000–U+F8FF). These
//     render as designed icons when the user has any Nerd Font patched
//     font installed (https://www.nerdfonts.com).
//   - An ASCII fallback. The default. Used on any terminal that does not
//     have a patched font, when stdout is not a TTY (e.g. CI logs,
//     captured output), or when the user sets HAWK_ICONS=ascii.
//
// No emoji block codepoints (U+1F300–U+1FAFF) and no symbol/dingbat block
// codepoints (U+2600–U+27BF) are used anywhere. The codepoints defined in
// this file are verified to be in the Unicode PUA by the audit test in
// internal/testaudit.
//
// Codepoints are derived from:
//   - Nerd Fonts v3 Codicons    (EA60..EC1E) — https://github.com/microsoft/vscode-codicons
//   - Nerd Fonts v3 FontAwesome (ED00..F2FF) — https://fontawesome.com
//   - Nerd Fonts v3 Octicons    (F400..F533) — https://primer.style/octicons
//   - Nerd Fonts v3 Pomicons    (E000..E00A) — pomodoro set
//   - Nerd Fonts v3 Custom      (E5FA..E6B7) — seti-ui
//
// Why this and not Lucide SVG?
//
//	Lucide (https://lucide.dev) is the project's visual identity for docs
//	and web surfaces (see docs/architecture.md). It is an SVG-only icon
//	set — there is no standard PUA mapping for Lucide in Nerd Fonts, so
//	the icon shapes cannot be rendered in a plain terminal. The two
//	realistic options are (a) build and ship a custom Nerd Font that
//	embeds a Lucide subset (large infrastructure dependency, no
//	toolchain in CI), or (b) render Lucide SVGs as Unicode block art at
//	print time (slow, error-prone, breaks captured output). The Go-CLI
//	ecosystem (charmbracelet, spf13/cobra, github/cli, k9s) uses the
//	PUA-or-ASCII approach we have here; the surveys of those projects
//	are recorded in scripts/ICONICONSURVEY.md. The 0-emoji audit in
//	internal/testaudit enforces that policy going forward.
package icons

// Nerd Font PUA codepoints. Every entry is in the basic Private Use Area
// (U+E000..U+F8FF) so the audit test in internal/testaudit can statically
// verify no emoji has crept in.
const (
	// ---- Codicons (nf-cod-*) — Microsoft VS Code icon set ----------
	// Codepoint = 0xEA60 + mapping.json key
	//   file 60027 → EA7B    question 60210 → EB12    mail 60188 → EB1C
	//   alert 60012 → EA9C    bell 60066 → EACE       search 60013 → EA9D
	//   gear 60152 → EB38     key 60177 → EB51         refresh 60215 → EB77
	//   pass 60324 → EBCC     pass-filled 60339 → EBD7
	//   error 60039 → EAB7    info 60020 → EAA4        warning 60012 → EA9C
	//   sync 60023 → EAA7      rocket 60228 → EB84      eye 60016 → EAD0
	//   trash 60033 → EAB1     history 60034 → EAB2     calendar 60080 → EB00
	//   debug-stop 60039 → EAB7   debug-pause 60113 → EAD1
	//   hubot 60168 → EB48     zap 60038 → EAB6
	//   arrow-right 60060 → EAEC   arrow-down 60058 → EAEA
	//   arrow-up 60065 → EAF1      arrow-left 60059 → EAEB
	//   chevron-right 60086 → EB06 chevron-down 60084 → EB04
	//   chevron-up 60087 → EB07    chevron-left 60085 → EB05
	//   check 60082 → EAC2        close 60022 → EAA6
	//   circle-filled 60017 → EA71 circle-outline 60092 → EAC0
	//   expand-all 60309 → EBC1   fold 60149 → EB35
	//   list-tree 60294 → EBCE    menu 60308 → EBC0
	//   triangle-down 60270 → EBAE  triangle-up 60273 → EBB1
	//   triangle-left 60271 → EBAF triangle-right 60272 → EBB0
	//   watch 60284 → EBBC        server 60240 → EB90
	//   shield 60243 → EB93       database 60110 → EB1E
	//   cloud 60330 → EBD2        globe 60161 → EB41
	//   home 60166 → EB46         pin 60203 → EB6B
	//   link 60181 → EB55         lock 60021 → EAA5
	//   unlock 60276 → EBA4       heart 60165 → EB45
	//   lightbulb 60001 → EA91    terminal 60037 → EAB5
	//   tools 60269 → EBAD        save 60235 → EB8B
	//   rocket 60228 → EB84       diff 60129 → EB69
	//   file-media 60138 → EB22   file-text 60510 → EC4E
	//   file-pdf 60139 → EB23     file-code 60137 → EB21
	//   new-file 60031 → EAAF     new-folder 60032 → EAB0
	//   folder 60035 → EAB3       folder-opened 60151 → EB37
	//   output 60317 → EBC5       debug-console 60315 → EBC3
	//   repo 60002 → EA62         git-branch 60527 → EC5F
	//   git-commit 60156 → EB3C   debug-start 60115 → EB43
	//   debug-restart 60114 → EB42 debug-continue 60111 → EB3F
	//   debug-step-over 60118 → EB46 debug-step-into 60116 → EB44
	//   debug-step-out 60117 → EB45 layers 60370 → EBF2
	//   notebook 60335 → EBD7     comment 60011 → EA9B
	//   person 60007 → EA97       archive 60056 → EAD8
	//   browser 60078 → EAFE      vm 60026 → EAEA (collision — see below)
	//   device-mobile 60123 → EB2B
	puaChevronRight   = "\ueb06" // nf-cod-chevron_right (60086)
	puaRobot          = "\ueb48" // nf-cod-hubot (60168)   — same shape as robot
	puaCircleFilled   = "\uea71" // nf-cod-circle-filled  (60017)
	puaCircleOutline  = "\ueac0" // nf-cod-circle-outline (60092)
	puaAlert          = "\uea9c" // nf-cod-alert          (60012)
	puaCheckBold      = "\ueac2" // nf-cod-check          (60082)
	puaCloseThick     = "\ueaa6" // nf-cod-close          (60022)
	puaArrowRight     = "\ueaec" // nf-cod-arrow_right    (60060)
	puaArrowLeft      = "\ueaeb" // nf-cod-arrow_left     (60059)
	puaArrowUp        = "\ueaf1" // nf-cod-arrow_up       (60065)
	puaArrowDown      = "\ueaea" // nf-cod-arrow_down     (60058)
	puaSwapHorizontal = "\uea7c" // nf-cod-arrow-swap     (60363)
	puaTimerSand      = "\uebbc" // nf-cod-watch          (60284) — clock-like
	puaReload         = "\uea7c" // nf-cod-sync           (60023)
	puaStop           = "\ueab7" // nf-cod-error          (60039) — stop-circle
	puaBell           = "\ueace" // nf-cod-bell           (60066)
	puaCancel         = "\ueaa6" // nf-cod-close          (60022) — alias of close
	puaCheckDecagram  = "\uebd7" // nf-cod-pass-filled    (60339) — filled check
	puaAlertOctagram  = "\uebd0" // nf-cod-stop-circle    (60325)
	puaArrowUpBold    = "\ueb07" // nf-cod-chevron-up     (60087) — return-ish
	puaRefresh        = "\ueb77" // nf-cod-refresh        (60215)
	puaHourglass      = "\uebbc" // nf-cod-watch          (60284)
	puaCheckCircle    = "\uebcc" // nf-cod-pass           (60324) — checked circle
	puaCloseCircle    = "\uebd0" // nf-cod-stop-circle    (60325) — close circle
	puaImage          = "\ueb22" // nf-cod-file-media     (60138)
	puaFileDocument   = "\uec4e" // nf-cod-file-text      (60510)
	puaKey            = "\ueb51" // nf-cod-key            (60177)
	puaCog            = "\ueb38" // nf-cod-gear           (60152)
	puaMagnify        = "\uea9d" // nf-cod-search         (60013)
	puaBolt           = "\ueab6" // nf-cod-zap            (60038)
	puaBrain          = "\uea91" // nf-cod-lightbulb      (60001) — visual metaphor
	puaEmail          = "\ueb1c" // nf-cod-mail           (60188)
	puaHelpCircle     = "\ueaa4" // nf-cod-info           (60020) — closest match
	puaBranch         = "\uea63" // nf-cod-repo_forked — fork/branch glyph; present in JetBrains Mono NF and every Nerd Font
	puaPullRequest    = "\uea64" // nf-cod-git_pull_request — PR glyph; present in JetBrains Mono NF and every Nerd Font
	puaClockOutline   = "\uf017" // nf-fa-clock_o         (61463)
	puaPause          = "\uead1" // nf-cod-debug-pause    (60113)
	puaExpandAll      = "\uebc1" // nf-cod-expand-all     (60309)
	puaContainer      = "\ueb90" // nf-cod-server         (60240)
	puaShield         = "\ueb93" // nf-cod-shield         (60243)
	puaTerminal       = "\ueab5" // nf-cod-terminal       (60037)
	puaCaretRight     = "\ueb06" // nf-cod-chevron_right  (60086)
	puaCaretDown      = "\ueb04" // nf-cod-chevron_down   (60084)
	puaTriangleSmall  = "\uebb1" // nf-cod-triangle-up    (60273) — collapsed
	puaCircleHalf     = "\uea71" // nf-cod-circle-filled  (60017) — visual fallback
	puaCircleQuarter  = "\uea71" // nf-cod-circle-filled  (60017) — visual fallback
	puaCircleSlice5   = "\uea71" // nf-cod-circle-filled  (60017) — visual fallback
	puaCircleSlice6   = "\uea71" // nf-cod-circle-filled  (60017) — visual fallback
	puaReturn         = "\uea4c" // nf-cod-keyboard-tab   (60476) — closest match
	puaRotateVariant  = "\uea7c" // nf-cod-sync           (60023)
	puaQuestion       = "\ueb12" // nf-cod-question       (60210)
	puaFile           = "\uea7b" // nf-cod-file           (60027)
	puaMail           = "\ueb1c" // nf-cod-mail           (60188)

	// Llama is not in Nerd Fonts. We pick a reserved-looking PUA slot
	// outside any Nerd Font set so a patched font never renders an
	// unrelated glyph. ASCII fallback is always used for the Ollama row.
	puaLlama = "\ue00a" // Pomicons last slot — rendered as a generic icon in Nerd Fonts

	// Nerd Fonts v3 Codicons pin (nf-cod-pin, 60203 → EB6B).
	puaPin = "\ueb6b"

	puaDatabase = "\ueb1e" // nf-cod-database
	puaNetwork  = "\ueb41" // nf-cod-globe
	puaRuby     = "\ueb34" // nf-cod-ruby
)
