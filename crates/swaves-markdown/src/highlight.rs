use comrak::plugins::syntect::SyntectAdapter;

pub fn adapter() -> SyntectAdapter {
    SyntectAdapter::new(Some("InspiredGitHub"))
}
