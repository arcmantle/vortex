var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();

app.MapGet("/", () => "Hello from Server B!");
app.MapGet("/health", () => Results.Ok(new { status = "healthy", server = "B" }));

app.Run();
