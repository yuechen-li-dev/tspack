const port = Number(Bun.env.PORT || "4172");
const root = new URL("../public/", import.meta.url);

Bun.serve({
  hostname: "127.0.0.1",
  port,
  async fetch(request) {
    const url = new URL(request.url);
    const pathname = url.pathname === "/" ? "/index.html" : url.pathname;
    const file = Bun.file(new URL(`.${pathname}`, root));

    if (!(await file.exists())) {
      return new Response("not found", { status: 404 });
    }

    return new Response(file);
  },
});

console.log(`ready bun-server http://127.0.0.1:${port}`);
