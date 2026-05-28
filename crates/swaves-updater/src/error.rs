use thiserror::Error;

#[derive(Debug, Error)]
pub enum Error {
    #[error("release version is required")]
    MissingVersion,
    #[error("archive name is required")]
    MissingArchiveName,
    #[error("archive url is required")]
    MissingArchiveUrl,
    #[error("archive path is required")]
    MissingArchivePath,
    #[error("runtime executable is required")]
    MissingRuntimeExecutable,
    #[error(transparent)]
    Supervisor(#[from] swaves_supervisor::Error),
}

pub type Result<T> = std::result::Result<T, Error>;
