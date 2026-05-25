mod app;
mod db;
mod routes;
mod view;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let state = app::AppState::bootstrap()?;
    let app = app::build_router(state.clone());
    let listener = tokio::net::TcpListener::bind(&state.listen_addr).await?;
    println!("rust-poc listening on {}", state.listen_addr);
    axum::serve(listener, app).await?;
    Ok(())
}
