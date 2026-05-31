const port = 4173;
const root = new URL("../public/", import.meta.url);

Deno.serve({ hostname: "127.0.0.1", port }, async (request) => {
  const url = new URL(request.url);
  const pathname = url.pathname === "/" ? "/index.html" : url.pathname;
  const fileUrl = new URL(`.${pathname}`, root);

  try {
    const content = await Deno.readFile(fileUrl);
    return new Response(content, { headers: { "content-type": contentType(pathname) } });
  } catch {
    return new Response("not found", { status: 404 });
  }
});

console.log(`ready deno-server http://127.0.0.1:${port}`);

function contentType(pathname: string): string {
  if (pathname.endsWith(".html")) {
    return "text/html; charset=utf-8";
  }
  if (pathname.endsWith(".js")) {
    return "text/javascript; charset=utf-8";
  }
  if (pathname.endsWith(".css")) {
    return "text/css; charset=utf-8";
  }
  return "text/plain; charset=utf-8";
}
