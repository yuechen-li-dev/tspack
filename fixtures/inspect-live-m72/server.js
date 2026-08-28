const http = require("node:http");

const port = Number(process.env.PORT ?? "5198");
const html = `<!doctype html>
<html>
  <body>
    <main
      role="main"
      aria-label="M72 live inspection"
      data-tspack-source="src/App.tsx:4:5"
      data-tspack-component="App"
      data-tspack-symbol="App"
    >
      <button
        data-tspack-source="src/Button.tsx:4:5"
        data-tspack-component="Button"
        data-tspack-symbol="Button.Primary"
      >Save</button>
      <section
        role="alert"
        aria-label="Save failed"
        data-tspack-source="src/Toast.tsx:4:5"
        data-tspack-component="Toast"
        data-tspack-symbol="Toast.Error"
      >
        <span>Unable to save</span>
        <button
          disabled
          data-tspack-source="src/Toast.tsx:6:7"
          data-tspack-component="Toast"
          data-tspack-symbol="Toast.DismissButton"
        >Dismiss</button>
      </section>
    </main>
  </body>
</html>`;

const server = http.createServer((request, response) => {
  response.setHeader("content-type", "text/html; charset=utf-8");
  response.end(html);
});

server.listen(port, "127.0.0.1");

function stop() {
  server.close(() => process.exit(0));
}

process.on("SIGINT", stop);
process.on("SIGTERM", stop);
