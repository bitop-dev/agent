//! file_info — Rust plugin tool
//!
//! Provides detailed file and directory information:
//! file size, line/word/character counts, MIME type guess,
//! and directory entry listing with totals.
//!
//! Zero external crates — uses only the Rust standard library.
//!
//! Protocol:
//!   in:  {"type":"describe"}
//!   out: {"name":"file_info","description":"...","parameters":{...}}
//!
//!   in:  {"type":"call","call_id":"c1","params":{"path":"src/main.rs"}}
//!   out: {"content":[{"type":"text","text":"..."}],"error":false}
//!
//! Build:
//!   cargo build --release
//!   ./target/release/file_info
//!
//! Test:
//!   echo '{"type":"describe"}' | cargo run -q

use std::collections::HashMap;
use std::fmt::Write as FmtWrite;
use std::fs;
use std::io::{self, BufRead, Write};
use std::path::Path;
use std::time::{SystemTime, UNIX_EPOCH};

// ── JSON helpers ─────────────────────────────────────────────────────────────
// We parse JSON manually to avoid external deps.

fn json_str(s: &str) -> String {
    let escaped = s
        .replace('\\', "\\\\")
        .replace('"', "\\\"")
        .replace('\n', "\\n")
        .replace('\r', "\\r")
        .replace('\t', "\\t");
    format!("\"{}\"", escaped)
}

fn json_text_result(text: &str, error: bool) -> String {
    format!(
        "{{\"content\":[{{\"type\":\"text\",\"text\":{}}}],\"error\":{}}}",
        json_str(text),
        error
    )
}

fn get_str<'a>(map: &'a HashMap<String, JsonVal>, key: &str) -> Option<&'a str> {
    match map.get(key) {
        Some(JsonVal::Str(s)) => Some(s.as_str()),
        _ => None,
    }
}

// ── Minimal JSON parser ───────────────────────────────────────────────────────

#[derive(Debug, Clone)]
enum JsonVal {
    Str(String),
    Num(f64),
    Bool(bool),
    Null,
    Obj(HashMap<String, JsonVal>),
    Arr(Vec<JsonVal>),
}

fn skip_ws(s: &[u8], pos: &mut usize) {
    while *pos < s.len() && (s[*pos] == b' ' || s[*pos] == b'\t' || s[*pos] == b'\n' || s[*pos] == b'\r') {
        *pos += 1;
    }
}

fn parse_str(s: &[u8], pos: &mut usize) -> Result<String, &'static str> {
    if s.get(*pos) != Some(&b'"') {
        return Err("expected '\"'");
    }
    *pos += 1;
    let mut out = String::new();
    while *pos < s.len() {
        match s[*pos] {
            b'"' => { *pos += 1; return Ok(out); }
            b'\\' => {
                *pos += 1;
                match s.get(*pos) {
                    Some(b'"') => { out.push('"'); *pos += 1; }
                    Some(b'\\') => { out.push('\\'); *pos += 1; }
                    Some(b'n') => { out.push('\n'); *pos += 1; }
                    Some(b'r') => { out.push('\r'); *pos += 1; }
                    Some(b't') => { out.push('\t'); *pos += 1; }
                    _ => { out.push('\\'); }
                }
            }
            c => { out.push(c as char); *pos += 1; }
        }
    }
    Err("unterminated string")
}

fn parse_val(s: &[u8], pos: &mut usize) -> Result<JsonVal, &'static str> {
    skip_ws(s, pos);
    match s.get(*pos) {
        Some(b'"') => Ok(JsonVal::Str(parse_str(s, pos)?)),
        Some(b'{') => {
            *pos += 1;
            let mut map = HashMap::new();
            skip_ws(s, pos);
            if s.get(*pos) == Some(&b'}') { *pos += 1; return Ok(JsonVal::Obj(map)); }
            loop {
                skip_ws(s, pos);
                let k = parse_str(s, pos)?;
                skip_ws(s, pos);
                if s.get(*pos) != Some(&b':') { return Err("expected ':'"); }
                *pos += 1;
                let v = parse_val(s, pos)?;
                map.insert(k, v);
                skip_ws(s, pos);
                match s.get(*pos) {
                    Some(b',') => { *pos += 1; }
                    Some(b'}') => { *pos += 1; break; }
                    _ => return Err("expected ',' or '}'"),
                }
            }
            Ok(JsonVal::Obj(map))
        }
        Some(b'[') => {
            *pos += 1;
            let mut arr = Vec::new();
            skip_ws(s, pos);
            if s.get(*pos) == Some(&b']') { *pos += 1; return Ok(JsonVal::Arr(arr)); }
            loop {
                arr.push(parse_val(s, pos)?);
                skip_ws(s, pos);
                match s.get(*pos) {
                    Some(b',') => { *pos += 1; }
                    Some(b']') => { *pos += 1; break; }
                    _ => return Err("expected ',' or ']'"),
                }
            }
            Ok(JsonVal::Arr(arr))
        }
        Some(b't') => { *pos += 4; Ok(JsonVal::Bool(true)) }
        Some(b'f') => { *pos += 5; Ok(JsonVal::Bool(false)) }
        Some(b'n') => { *pos += 4; Ok(JsonVal::Null) }
        _ => {
            let start = *pos;
            while *pos < s.len() && !matches!(s[*pos], b',' | b'}' | b']' | b' ' | b'\t' | b'\n') {
                *pos += 1;
            }
            let tok = std::str::from_utf8(&s[start..*pos]).unwrap_or("0");
            tok.parse::<f64>().map(JsonVal::Num).map_err(|_| "invalid token")
        }
    }
}

fn parse_obj(line: &str) -> Result<HashMap<String, JsonVal>, String> {
    let bytes = line.as_bytes();
    let mut pos = 0;
    match parse_val(bytes, &mut pos) {
        Ok(JsonVal::Obj(m)) => Ok(m),
        Ok(_) => Err("expected JSON object".into()),
        Err(e) => Err(e.to_string()),
    }
}

// ── Tool logic ────────────────────────────────────────────────────────────────

fn format_size(bytes: u64) -> String {
    if bytes < 1024 { return format!("{} B", bytes); }
    if bytes < 1024 * 1024 { return format!("{:.1} KB", bytes as f64 / 1024.0); }
    format!("{:.1} MB", bytes as f64 / (1024.0 * 1024.0))
}

fn format_mtime(mtime: SystemTime) -> String {
    match mtime.duration_since(UNIX_EPOCH) {
        Ok(d) => format!("{}s since epoch", d.as_secs()),
        Err(_) => "unknown".to_string(),
    }
}

fn guess_mime(path: &Path) -> &'static str {
    match path.extension().and_then(|e| e.to_str()) {
        Some("rs") => "text/x-rust",
        Some("go") => "text/x-go",
        Some("py") => "text/x-python",
        Some("ts") | Some("js") => "text/javascript",
        Some("json") => "application/json",
        Some("yaml") | Some("yml") => "text/yaml",
        Some("toml") => "text/toml",
        Some("md") => "text/markdown",
        Some("html") | Some("htm") => "text/html",
        Some("sh") | Some("bash") => "text/x-shellscript",
        Some("rb") => "text/x-ruby",
        Some("txt") => "text/plain",
        Some("png") => "image/png",
        Some("jpg") | Some("jpeg") => "image/jpeg",
        Some("gif") => "image/gif",
        Some("pdf") => "application/pdf",
        Some("zip") => "application/zip",
        _ => "application/octet-stream",
    }
}

fn info_file(path: &Path) -> Result<String, String> {
    let meta = fs::metadata(path).map_err(|e| format!("cannot stat {}: {}", path.display(), e))?;
    let size = meta.len();
    let mime = guess_mime(path);
    let mtime = meta.modified().unwrap_or(UNIX_EPOCH);

    let mut out = String::new();
    writeln!(out, "path    : {}", path.display()).unwrap();
    writeln!(out, "type    : file").unwrap();
    writeln!(out, "size    : {} ({})", size, format_size(size)).unwrap();
    writeln!(out, "mime    : {}", mime).unwrap();
    writeln!(out, "modified: {}", format_mtime(mtime)).unwrap();

    // Count lines/words/chars for text files
    if mime.starts_with("text/") || mime == "application/json" {
        match fs::read_to_string(path) {
            Ok(content) => {
                let lines = content.lines().count();
                let words = content.split_whitespace().count();
                let chars = content.chars().count();
                writeln!(out, "lines   : {}", lines).unwrap();
                writeln!(out, "words   : {}", words).unwrap();
                writeln!(out, "chars   : {}", chars).unwrap();
            }
            Err(_) => {
                writeln!(out, "lines   : (binary or unreadable)").unwrap();
            }
        }
    }

    Ok(out.trim_end().to_string())
}

fn info_dir(path: &Path, max_entries: usize) -> Result<String, String> {
    let meta = fs::metadata(path).map_err(|e| format!("cannot stat {}: {}", path.display(), e))?;
    let mtime = meta.modified().unwrap_or(UNIX_EPOCH);

    let entries = fs::read_dir(path).map_err(|e| format!("cannot read dir: {}", e))?;
    let mut items: Vec<(String, bool, u64)> = Vec::new(); // (name, is_dir, size)
    let mut total_size: u64 = 0;

    for entry in entries.flatten() {
        let name = entry.file_name().to_string_lossy().to_string();
        let meta = entry.metadata().ok();
        let is_dir = meta.as_ref().map(|m| m.is_dir()).unwrap_or(false);
        let size = meta.as_ref().map(|m| m.len()).unwrap_or(0);
        total_size += size;
        items.push((name, is_dir, size));
    }
    items.sort_by(|a, b| {
        // Dirs first, then alphabetical
        b.1.cmp(&a.1).then(a.0.cmp(&b.0))
    });

    let mut out = String::new();
    writeln!(out, "path     : {}", path.display()).unwrap();
    writeln!(out, "type     : directory").unwrap();
    writeln!(out, "entries  : {}", items.len()).unwrap();
    writeln!(out, "total    : {}", format_size(total_size)).unwrap();
    writeln!(out, "modified : {}", format_mtime(mtime)).unwrap();
    writeln!(out).unwrap();

    let show = items.len().min(max_entries);
    for (name, is_dir, size) in &items[..show] {
        let suffix = if *is_dir { "/" } else { "" };
        writeln!(out, "  {:42} {:>10}", format!("{}{}", name, suffix), format_size(*size)).unwrap();
    }
    if items.len() > max_entries {
        writeln!(out, "  … {} more entries", items.len() - max_entries).unwrap();
    }

    Ok(out.trim_end().to_string())
}

fn handle_call(params: &HashMap<String, JsonVal>) -> (String, bool) {
    let path_str = match get_str(params, "path") {
        Some(p) => p,
        None => return ("Error: 'path' parameter is required".into(), true),
    };

    let max_entries = match params.get("max_entries") {
        Some(JsonVal::Num(n)) => (*n as usize).max(1).min(500),
        _ => 50,
    };

    let path = Path::new(path_str);
    if !path.exists() {
        return (format!("Error: path not found: {}", path_str), true);
    }

    let result = if path.is_dir() {
        info_dir(path, max_entries)
    } else {
        info_file(path)
    };

    match result {
        Ok(text) => (text, false),
        Err(e) => (format!("Error: {}", e), true),
    }
}

// ── Main ──────────────────────────────────────────────────────────────────────

const DEFINITION: &str = r#"{"name":"file_info","description":"Get detailed metadata and statistics about a file or directory. For files: size, MIME type, line/word/character count. For directories: entry listing with sizes. Useful for understanding the contents of a path before reading it.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"File or directory path to inspect"},"max_entries":{"type":"integer","description":"Maximum directory entries to list (default: 50, max: 500)"}},"required":["path"]}}"#;

fn main() {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut out = stdout.lock();

    for line in stdin.lock().lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => break,
        };
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }

        let response = match parse_obj(trimmed) {
            Err(e) => json_text_result(&format!("JSON parse error: {}", e), true),
            Ok(msg) => {
                match get_str(&msg, "type") {
                    Some("describe") => DEFINITION.to_string(),
                    Some("call") => {
                        let params = match msg.get("params") {
                            Some(JsonVal::Obj(p)) => p.clone(),
                            _ => HashMap::new(),
                        };
                        let (text, is_error) = handle_call(&params);
                        json_text_result(&text, is_error)
                    }
                    Some(t) => json_text_result(&format!("Unknown type: {}", t), true),
                    None => json_text_result("Missing 'type' field", true),
                }
            }
        };

        writeln!(out, "{}", response).unwrap();
        out.flush().unwrap();
    }
}
