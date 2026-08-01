var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();

app.MapGet("/api/status", () => Results.Json(new { status = "ok" }));

app.Run();
