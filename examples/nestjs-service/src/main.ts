import "reflect-metadata";

import { NestFactory } from "@nestjs/core";
import { AppModule } from "./app.module";

function readPort(): number {
  const rawPort = process.env.PORT ?? "3000";
  const parsedPort = Number.parseInt(rawPort, 10);

  if (Number.isNaN(parsedPort) || parsedPort <= 0) {
    throw new Error(`PORT must be a positive integer, received ${rawPort}`);
  }

  return parsedPort;
}

async function bootstrap(): Promise<void> {
  const app = await NestFactory.create(AppModule);
  const port = readPort();

  await app.listen(port, "127.0.0.1");
  console.log(`NestJS service listening on http://127.0.0.1:${port}`);
}

void bootstrap();
