use std::collections::BTreeMap;

use serde_json::{Map, Value};
use serde_yaml::Value as YamlValue;

use crate::error::{Error, Result};

#[derive(Debug, Clone, PartialEq)]
pub struct ParsedSource {
    pub metadata: BTreeMap<String, Value>,
    pub markdown: String,
}

pub fn split(input: &str) -> Result<ParsedSource> {
    let Some((front_matter, body)) = extract_front_matter(input) else {
        return Ok(ParsedSource {
            metadata: BTreeMap::new(),
            markdown: input.trim().to_string(),
        });
    };

    let metadata = parse_metadata(front_matter)?;
    Ok(ParsedSource {
        metadata,
        markdown: body.trim().to_string(),
    })
}

fn extract_front_matter(input: &str) -> Option<(&str, &str)> {
    let (first_line, mut cursor) = next_line(input, 0)?;
    if first_line.trim_end() != "---" {
        return None;
    }

    let start = cursor;
    while let Some((line, next)) = next_line(input, cursor) {
        if line.trim_end() == "---" {
            return Some((&input[start..cursor], &input[next..]));
        }
        cursor = next;
    }

    None
}

fn next_line(input: &str, start: usize) -> Option<(&str, usize)> {
    if start >= input.len() {
        return None;
    }

    let rest = &input[start..];
    match rest.find('\n') {
        Some(offset) => {
            let end = start + offset;
            let line = input[start..end].trim_end_matches('\r');
            Some((line, end + 1))
        }
        None => Some((rest.trim_end_matches('\r'), input.len())),
    }
}

fn parse_metadata(front_matter: &str) -> Result<BTreeMap<String, Value>> {
    if front_matter.trim().is_empty() {
        return Ok(BTreeMap::new());
    }

    let value: YamlValue = serde_yaml::from_str(front_matter)?;
    let mapping = match value {
        YamlValue::Mapping(mapping) => mapping,
        YamlValue::Null => return Ok(BTreeMap::new()),
        _ => return Err(Error::FrontMatterNotMapping),
    };

    let mut metadata = BTreeMap::new();
    for (key, value) in mapping {
        let key = yaml_key_to_string(&key)?;
        metadata.insert(key, yaml_to_json(value)?);
    }

    Ok(metadata)
}

fn yaml_key_to_string(value: &YamlValue) -> Result<String> {
    match value {
        YamlValue::Null => Ok("null".to_string()),
        YamlValue::Bool(value) => Ok(value.to_string()),
        YamlValue::Number(value) => Ok(value.to_string()),
        YamlValue::String(value) => Ok(value.clone()),
        _ => Err(Error::FrontMatterKey),
    }
}

fn yaml_to_json(value: YamlValue) -> Result<Value> {
    Ok(match value {
        YamlValue::Null => Value::Null,
        YamlValue::Bool(value) => Value::Bool(value),
        YamlValue::Number(number) => {
            serde_json::to_value(number).map_err(|error| Error::Math(error.to_string()))?
        }
        YamlValue::String(value) => Value::String(value),
        YamlValue::Sequence(items) => Value::Array(
            items
                .into_iter()
                .map(yaml_to_json)
                .collect::<Result<Vec<_>>>()?,
        ),
        YamlValue::Mapping(mapping) => {
            let mut object = Map::new();
            for (key, value) in mapping {
                object.insert(yaml_key_to_string(&key)?, yaml_to_json(value)?);
            }
            Value::Object(object)
        }
        YamlValue::Tagged(tagged) => yaml_to_json(tagged.value)?,
    })
}
