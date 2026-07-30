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
