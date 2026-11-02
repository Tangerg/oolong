package ansi

// SGR parameters are shared wire vocabulary. Renderers and decoders interpret
// them independently, but must not assign different numbers to the same command.
const (
	SGRReset                 = 0
	SGRBold                  = 1
	SGRFaint                 = 2
	SGRItalic                = 3
	SGRUnderline             = 4
	SGRReverse               = 7
	SGRStrike                = 9
	SGRDoubleUnderline       = 21
	SGRNormalIntensity       = 22
	SGRNotItalic             = 23
	SGRNotUnderlined         = 24
	SGRNotReversed           = 27
	SGRNotStruckOut          = 29
	SGRForegroundFirst       = 30
	SGRForegroundLast        = 37
	SGRForeground            = 38
	SGRForegroundDefault     = 39
	SGRBackgroundFirst       = 40
	SGRBackgroundLast        = 47
	SGRBackground            = 48
	SGRBackgroundDefault     = 49
	SGRUnderlineColor        = 58
	SGRBrightForegroundFirst = 90
	SGRBrightForegroundLast  = 97
	SGRBrightBackgroundFirst = 100
	SGRBrightBackgroundLast  = 107
)

// Subparameters have their own namespace: their numeric overlap with top-level
// attributes must not make a color mode change an attribute.
const (
	SGRColorRGB        = 2
	SGRColorIndexed    = 5
	SGRNoUnderline     = 0
	ColorsPerIntensity = 8
)
