use crate::{Error, Result};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ReleaseAsset {
    version: String,
    archive_name: String,
    archive_url: String,
    checksum_url: Option<String>,
}

impl ReleaseAsset {
    pub fn new(
        version: impl Into<String>,
        archive_name: impl Into<String>,
        archive_url: impl Into<String>,
        checksum_url: Option<String>,
    ) -> Result<Self> {
        let version = version.into().trim().to_owned();
        if version.is_empty() {
            return Err(Error::MissingVersion);
        }

        let archive_name = archive_name.into().trim().to_owned();
        if archive_name.is_empty() {
            return Err(Error::MissingArchiveName);
        }

        let archive_url = archive_url.into().trim().to_owned();
        if archive_url.is_empty() {
            return Err(Error::MissingArchiveUrl);
        }

        Ok(Self {
            version,
            archive_name,
            archive_url,
            checksum_url: checksum_url
                .map(|value| value.trim().to_owned())
                .filter(|value| !value.is_empty()),
        })
    }

    pub fn version(&self) -> &str {
        &self.version
    }

    pub fn archive_name(&self) -> &str {
        &self.archive_name
    }

    pub fn archive_url(&self) -> &str {
        &self.archive_url
    }

    pub fn checksum_url(&self) -> Option<&str> {
        self.checksum_url.as_deref()
    }
}
