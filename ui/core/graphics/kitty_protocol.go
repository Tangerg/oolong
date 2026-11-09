package graphics

const (
	graphicsCommand = 'G'
	kittyStart      = "\x1b_" + string(graphicsCommand)
	kittyEnd        = "\x1b\\"
	wireIntegerBits = 32

	imageIDField      = "i"
	imageNumberField  = "I"
	placementField    = "p"
	actionField       = "a"
	formatField       = "f"
	quietField        = "q"
	moreField         = "m"
	deleteField       = "d"
	transmissionField = "t"
	columnsField      = "c"
	rowsField         = "r"
	layerField        = "z"
	cursorPolicyField = "C"
	sourceXField      = "x"
	sourceYField      = "y"
	sourceWidthField  = "w"
	sourceHeightField = "h"

	transmitAction = "t"
	placeAction    = "p"
	deleteAction   = "d"
	queryAction    = "q"

	deletePlacement = "i"
	deleteImageData = "I"
	directData      = "d"
)

const (
	transmitFormat = kittyStart + actionField + "=" + transmitAction + "," + formatField + "=%d,%s," + moreField + "=%d;%s" + kittyEnd
	continueFormat = kittyStart + moreField + "=%d;%s" + kittyEnd
	placeFormat    = kittyStart + actionField + "=" + placeAction + "," + imageIDField + "=%d," + placementField + "=%d%s," +
		columnsField + "=%d," + rowsField + "=%d," + layerField + "=%d," + cursorPolicyField + "=%d," + quietField + "=%d;" + kittyEnd
	cropFormat  = "," + sourceXField + "=%d," + sourceYField + "=%d," + sourceWidthField + "=%d," + sourceHeightField + "=%d"
	eraseFormat = kittyStart + actionField + "=" + deleteAction + "," + deleteField + "=" + deletePlacement + "," +
		imageIDField + "=%d," + placementField + "=%d," + quietField + "=%d;" + kittyEnd
	releaseFormat = kittyStart + actionField + "=" + deleteAction + "," + deleteField + "=" + deleteImageData + "," +
		imageIDField + "=%d," + quietField + "=%d;" + kittyEnd
	queryFormat = kittyStart + imageIDField + "=%d," + actionField + "=" + queryAction + "," + transmissionField + "=" + directData + "," +
		formatField + "=%d;%s" + kittyEnd
)
