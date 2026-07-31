package tui

import "time"

// Banner is the ASCII art logo for MineOS
const Banner = `  __  __ _             ___  ____
 |  \/  (_)_ __   ___ / _ \/ ___|
 | |\/| | | '_ \ / _ \ | | \___ \
 | |  | | | | | |  __/ |_| |___) |
 |_|  |_|_|_| |_|\___|\___/|____/ `

const BannerTagline = "Minecraft Server Management"

const MinecraftCat = `                                                                                              :::::-
                                                         --                                 :::::-++
                                                     ===-------                          ------+**++
                                                 ==========-------=                   =----=+*****
                                              --================----=*              -----+******
                                           ===-----======++++====+****+           -----*****#
                                       ----===++==----===--=++*#***********    ----=+*****#
                                   +==--------========---=+*####*************+=--=****##
                                ====++===-----========+***######******##************#
                            =----===========-----==*####**######******     *******#
           -----=+    ----===+==-----==========+****####**######******       +**
         +==-=+*** +=--------======-------=+*###****####****####******
        ++++*****=====---=+*==----======****####****####****####******
    ==----=+**+=--=---=+++==--------+*#*****####****####****##********
=--=======---=====++++++++======+*#**##*****###******#****************
+==---=============++++==+**+***###**###****###**********************#
+++**+=----==========+*####%###*###**###****##*********************
-=+*****+=----===+*########%%##*###**###***************************
=-+##******+==+*###########%%##*###**###*******************++******
++=+#*++*#*++***#########*#%###*###*******************+*++===******
+=----=+*#*==+**#########*#%###*###***************+=---+===-=+*****
****+=---=+==+**####*####*#%%##*###********+*****====--+----=+*+***
++*****+***+++**####*****#%%%##*###**********     ===--+===-=+++*++
++++++*****++***##****+=+####***##*****##             -+===-=++++++
 +++++++***++******+=-===+************                    =-=+***
     +++***++**##====-===+************
                   ======+***+++******
                   ======++++==+******
                   ======+====-=****++
                   =----=+======++++++
                     ---=+======++++++
                          =----=++++*+
                             --=+**`

// Log buffer and streaming constants
const (
	MaxLogLines          = 5000 // Increased buffer size
	DefaultDockerLogTail = 200
	LogRetryDelay        = 2 * time.Second
	MaxLogRetries        = 3
	// LogBatchMax caps how many already-queued log lines are delivered in one
	// message. Bubble Tea re-renders the whole view per message, so handing it
	// one line at a time makes a startup burst crawl; batching turns a few
	// hundred renders into one. The cap keeps a runaway producer from starving
	// key handling.
	LogBatchMax = 256
)

// Streaming constants
const (
	StreamingBufferSize = 100
	ScannerMaxBuffer    = 1024 * 1024 // 1MB
)

// Performance history / sparkline constants
const (
	// PerfHistoryMinutes is the window requested when the metrics panel opens.
	// The API clamps to 5..1440.
	PerfHistoryMinutes = 60
	// MaxPerfHistory bounds the retained samples. At the stream's 2s cadence
	// this is a bit over half an hour of live data on top of the seed.
	MaxPerfHistory = 1200
	// SparklineWidth is how many columns a sparkline is drawn in.
	SparklineWidth = 24
	// LowTpsThreshold mirrors the server-side low-TPS alert in
	// PerformanceService, so the CLI highlights exactly what the API warns on.
	LowTpsThreshold = 18.0
)

// Timeout constants
const (
	HealthPollInterval = 10 * time.Second // Re-check API when unhealthy

	// How long a transient notice stays on the footer before expiring.
	// Errors linger longer than confirmations because they are worth reading
	// twice; both are replaced immediately by a newer notice.
	StatusMsgTTL = 6 * time.Second
	ErrorMsgTTL  = 20 * time.Second
)

// UI layout constants
const (
	SidebarWidth     = 20
	MinContentHeight = 5
	// Minimum terminal dimensions below which the TUI renders a "resize" notice
	// instead of a layout. Guards against negative content widths (which would
	// panic strings.Repeat). Tunable.
	MinTerminalWidth  = 40
	MinTerminalHeight = 10
)

// Default source for docker logs
const DefaultDockerLogSource = "all"

// Minecraft log types
var MinecraftLogTypes = []string{"combined", "server", "java", "crash"}
