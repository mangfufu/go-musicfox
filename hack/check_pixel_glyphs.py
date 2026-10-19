"""Check built-in UI literals against Fusion Pixel's actual cmap (fonttools required)."""

from pathlib import Path
import re
import sys

from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parents[1]
FONT = ROOT / "assets/fonts/fusion-pixel/fusion-pixel-12px-monospaced-zh_hans.otf"
ROOTS = (
    "internal", "cmd", "utils",
    "vendor/github.com/anhoder/foxful-cli",
    "vendor/charm.land/glamour/v2",
    "vendor/charm.land/bubbles/v2",
    "vendor/charm.land/lipgloss/v2",
)
# These characters are accepted/removed as input, not rendered as UI literals.
INPUT_ONLY = {
    "internal/keybindings/keybindings.go": {0x2212},
    "utils/filex/embed/config.toml": {0x2212},
    "internal/lyric/yrc.go": {0xFEFF},
}
LITERALS = re.compile(
    r'//[^\n]*|/\*.*?\*/|\#[^\n]*|`[^`]*`|"(?:\\.|[^"\\])*"|\x27(?:\\.|[^\x27\\])*\x27',
    re.S,
)
ESCAPES = re.compile(r"\\U([0-9a-fA-F]{8})|\\u([0-9a-fA-F]{4})")


def main():
    cmap = TTFont(FONT).getBestCmap()
    failures, checked = [], 0
    for folder in ROOTS:
        for path in sorted((ROOT / folder).rglob("*")):
            if path.suffix not in (".go", ".toml") or path.name.endswith("_test.go"):
                continue
            checked += 1
            relative = path.relative_to(ROOT).as_posix()
            source = path.read_text(encoding="utf-8-sig")
            ignored = INPUT_ONLY.get(relative, set())
            for match in LITERALS.finditer(source):
                literal = match.group()
                if literal.startswith(("/", "#")):
                    continue
                value = ESCAPES.sub(lambda m: chr(int(m.group(1) or m.group(2), 16)), literal[1:-1])
                missing = sorted({ord(c) for c in value if ord(c) > 127 and ord(c) not in cmap
                                  and ord(c) not in ignored and not c.isspace()})
                if missing:
                    line = source.count("\n", 0, match.start()) + 1
                    failures.append(f"{relative}:{line}: " + ", ".join(f"U+{cp:04X}" for cp in missing))
    if failures:
        print("\n".join(failures))
        return 1
    print(f"PASS: {checked} source/config files; all built-in display literals covered by Fusion Pixel.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
