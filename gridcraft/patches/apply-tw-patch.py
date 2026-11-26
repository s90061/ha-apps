"""Patch storytold/gridcraft v0.19.1 to add Taiwan Traditional Chinese (Excel terminology)
and embed Noto Sans TC Regular for WebAssembly rendering."""

import pathlib
import shutil
import sys


def replace_once(path: pathlib.Path, old: str, new: str) -> None:
    text = path.read_text("utf-8")
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"Expected 1 occurrence in {path}, found {count}:\n{old[:160]}")
    path.write_text(text.replace(old, new, 1), "utf-8")


def main() -> None:
    if len(sys.argv) != 4:
        raise SystemExit("Usage: apply-tw-patch.py <gridcraft_src_dir> <patches_dir> <font_otf_path>")

    src = pathlib.Path(sys.argv[1]).resolve()
    patches = pathlib.Path(sys.argv[2]).resolve()
    font_otf = pathlib.Path(sys.argv[3]).resolve()

    if not font_otf.is_file():
        raise RuntimeError(f"Missing Taiwan TC font file: {font_otf}")

    # 1. Copy Taiwan Traditional Chinese translation table and font into ui-egui crate
    shutil.copy2(patches / "zh_tw.rs", src / "crates/ui-egui/src/i18n/zh_tw.rs")
    shutil.copy2(font_otf, src / "crates/ui-egui/src/theme/NotoSansTC-Regular.otf")

    # 2. Wire Language::ZhTw into crates/ui-egui/src/i18n.rs
    i18n_rs = src / "crates/ui-egui/src/i18n.rs"
    replace_once(i18n_rs, "mod zh;\n", "mod zh;\nmod zh_tw;\n")
    replace_once(
        i18n_rs,
        "    Zh,\n    Ja,\n",
        "    Zh,\n    #[serde(rename = \"zh-tw\")]\n    ZhTw,\n    Ja,\n",
    )
    replace_once(
        i18n_rs,
        "    pub const ALL: [Self; 19] = [\n",
        "    pub const ALL: [Self; 20] = [\n",
    )
    replace_once(
        i18n_rs,
        "        Self::Zh,\n        Self::Ja,\n",
        "        Self::Zh,\n        Self::ZhTw,\n        Self::Ja,\n",
    )
    replace_once(
        i18n_rs,
        '            Self::Zh => "简体中文",\n',
        '            Self::Zh => "简体中文",\n            Self::ZhTw => "繁體中文（台灣）",\n',
    )
    replace_once(
        i18n_rs,
        '            Self::Zh => "zh",\n',
        '            Self::Zh => "zh",\n            Self::ZhTw => "zh-tw",\n',
    )
    replace_once(
        i18n_rs,
        '''    fn from_tag(tag: &str) -> Option<Self> {
        match tag.trim().split(['-', '_']).next()?.to_ascii_lowercase().as_str() {
            "en" => Some(Self::En),
            "es" => Some(Self::Es),
            "el" => Some(Self::El),
            "zh" => Some(Self::Zh),''',
        '''    fn from_tag(tag: &str) -> Option<Self> {
        let lower = tag.trim().to_ascii_lowercase();
        let parts: Vec<&str> = lower.split(['-', '_', '.']).filter(|p| !p.is_empty()).collect();
        let primary = *parts.first()?;
        if primary == "zh" {
            if parts.iter().skip(1).any(|p| matches!(*p, "tw" | "hk" | "mo" | "hant")) {
                return Some(Self::ZhTw);
            }
            return Some(Self::Zh);
        }
        match primary {
            "en" => Some(Self::En),
            "es" => Some(Self::Es),
            "el" => Some(Self::El),''',
    )
    replace_once(
        i18n_rs,
        "            Self::Zh => zh::translate(text),\n",
        "            Self::Zh => zh::translate(text),\n            Self::ZhTw => zh_tw::translate(text),\n",
    )

    # 3. Embed NotoSansTC-Regular.otf in crates/ui-egui/src/theme/font_setup.rs
    font_setup_rs = src / "crates/ui-egui/src/theme/font_setup.rs"
    replace_once(
        font_setup_rs,
        "    append_craft_faces(&mut fonts);\n",
        "    append_embedded_tw_font(&mut fonts);\n    append_craft_faces(&mut fonts);\n",
    )
    replace_once(
        font_setup_rs,
        "/// Appends the permitted craft-fonts faces to every family",
        '''/// Embedded Noto Sans TC Regular (Taiwan Traditional Chinese, OFL-1.1) so WebAssembly builds
/// render crisp Taiwan Traditional Chinese UI labels and spreadsheet cell contents without tofu.
const HA_TW_FONT_KEY: &str = "ha-noto-sans-tc";
const HA_TW_FONT_BYTES: &[u8] = include_bytes!("NotoSansTC-Regular.otf");

fn append_embedded_tw_font(fonts: &mut FontDefinitions) {
    if !font_parses(HA_TW_FONT_BYTES, 0) {
        return;
    }
    let mut data = FontData::from_static(HA_TW_FONT_BYTES);
    data.tweak.set_baseline_fallback(true);
    fonts.font_data.insert(HA_TW_FONT_KEY.to_string(), Arc::new(data));
    for chain in fonts.families.values_mut() {
        if !chain.iter().any(|k| k == HA_TW_FONT_KEY) {
            chain.push(HA_TW_FONT_KEY.to_string());
        }
    }
}

/// Appends the permitted craft-fonts faces to every family''',
    )

    # 4. Support ?lang= query parameter on first visit in apps/gridcraft-web/src/web.rs
    #    (while still letting saved localStorage preferences take precedence).
    web_rs = src / "apps/gridcraft-web/src/web.rs"
    replace_once(
        web_rs,
        '''                    let browser = gridcraft_ui_egui::i18n::Language::system();
                    if !matches!(
                        browser,
                        gridcraft_ui_egui::i18n::Language::Zh | gridcraft_ui_egui::i18n::Language::Ja | gridcraft_ui_egui::i18n::Language::Ko
                    ) {
                        app.ui.language = browser;
                    }''',
        '''                    let browser = gridcraft_ui_egui::i18n::Language::system();
                    if !matches!(
                        browser,
                        gridcraft_ui_egui::i18n::Language::Zh | gridcraft_ui_egui::i18n::Language::Ja | gridcraft_ui_egui::i18n::Language::Ko
                    ) {
                        app.ui.language = browser;
                    }
                    if let Some(lang) = query_param("lang").and_then(|code| gridcraft_ui_egui::i18n::Language::parse(&code)) {
                        app.ui.language = lang;
                    }''',
    )

    print("Successfully applied Taiwan Traditional Chinese patch + NotoSansTC font to", src)


if __name__ == "__main__":
    main()
