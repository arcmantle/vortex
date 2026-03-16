var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();

app.MapGet("/", () => "Hello from Server A!");
app.MapGet("/health", () => Results.Ok(new { status = "healthy", server = "A" }));

app.Run();
