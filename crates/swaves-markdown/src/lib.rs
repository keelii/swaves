mod error;
mod frontmatter;
mod heading;
mod highlight;
mod math;
mod mermaid;
mod options;
mod render;
mod toc;
mod transform;
mod util;

pub use crate::error::{Error, Result};
pub use crate::frontmatter::{ParsedSource, split};
pub use crate::heading::{Heading, HeadingIdStrategy};
pub use crate::options::{RenderFailureMode, RenderOptions};
pub use crate::render::{RenderResult, TocResult, render, render_toc};
