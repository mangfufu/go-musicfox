//go:build windows

package pixelfont

import _ "embed"

const Name = "Fusion Pixel 12px Mono zh-Hans"

// Data is the unmodified OFL-licensed font, loaded privately by desktop lyrics.
//
//go:embed fusion-pixel-12px-monospaced-zh_hans.otf
var Data []byte
