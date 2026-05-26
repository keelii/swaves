use thiserror::Error;

#[derive(Debug, Error)]
pub enum Error {
    #[error("front matter must be a YAML mapping")]
    FrontMatterNotMapping,
    #[error("front matter key must be a string-like scalar")]
    FrontMatterKey,
    #[error("failed to parse front matter: {0}")]
    FrontMatterYaml(#[from] serde_yaml::Error),
    #[error("failed to format html output")]
    HtmlFormat(#[from] std::fmt::Error),
    #[error("failed to render math formula: {0}")]
    Math(String),
}

pub type Result<T> = std::result::Result<T, Error>;
