use std::{collections::BTreeMap, env, fs, path::PathBuf};

fn main() {
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR"));
    let go_consts = manifest_dir.join("../../internal/platform/db/consts.go");
    println!("cargo:rerun-if-changed={}", go_consts.display());

    let source = fs::read_to_string(&go_consts).expect("read Go db consts");
    let table_names = parse_table_names(&source);
    let initial_sql_expr =
        extract_initial_sql_expr(&source).expect("extract InitialSQL expression");
    let rendered_sql = render_go_concat_expr(initial_sql_expr, &table_names);

    assert!(
        rendered_sql.contains("CREATE TABLE IF NOT EXISTS t_tasks"),
        "rendered InitialSQL missing t_tasks"
    );
    assert!(
        rendered_sql.contains("CREATE TABLE IF NOT EXISTS t_settings"),
        "rendered InitialSQL missing t_settings"
    );

    let out_dir = PathBuf::from(env::var("OUT_DIR").expect("OUT_DIR"));
    fs::write(out_dir.join("initial_sql.sql"), rendered_sql).expect("write rendered InitialSQL");
}

fn parse_table_names(source: &str) -> BTreeMap<String, String> {
    let mut table_names = BTreeMap::new();

    for line in source.lines() {
        let trimmed = line.trim();
        if !trimmed.starts_with("Table") || !trimmed.contains("TableName = ") {
            continue;
        }

        let mut parts = trimmed.splitn(2, "TableName = ");
        let name = parts
            .next()
            .expect("table name prefix")
            .split_whitespace()
            .next()
            .expect("table const name");
        let raw_value = parts.next().expect("table name value").trim();
        let Some(value) = raw_value
            .strip_prefix('"')
            .and_then(|v| v.strip_suffix('"'))
        else {
            continue;
        };
        table_names.insert(name.to_string(), value.to_string());
    }

    table_names
}

fn extract_initial_sql_expr(source: &str) -> Option<&str> {
    let start = source.find("const InitialSQL = ")? + "const InitialSQL = ".len();
    let end = source[start..].find("\nconst InternalLang = `")? + start;
    Some(&source[start..end])
}

fn render_go_concat_expr(expr: &str, table_names: &BTreeMap<String, String>) -> String {
    let mut rendered = String::new();
    let mut chars = expr.char_indices().peekable();

    while let Some((_, ch)) = chars.next() {
        match ch {
            '`' => {
                while let Some((_, next_ch)) = chars.next() {
                    if next_ch == '`' {
                        break;
                    }
                    rendered.push(next_ch);
                }
            }
            c if c.is_ascii_alphabetic() || c == '_' => {
                let mut ident = String::from(c);
                while let Some((_, next_ch)) = chars.peek() {
                    if next_ch.is_ascii_alphanumeric() || *next_ch == '_' {
                        ident.push(*next_ch);
                        chars.next();
                    } else {
                        break;
                    }
                }
                if let Some(value) = table_names.get(&ident) {
                    rendered.push_str(value);
                }
            }
            _ => {}
        }
    }

    rendered
}
