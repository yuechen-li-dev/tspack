import { Injectable } from "@nestjs/common";

export interface RootResponse {
  message: string;
}

export interface HealthResponse {
  ok: true;
}

@Injectable()
export class AppService {
  getRoot(): RootResponse {
    return {
      message: "Hello from TSPack NestJS service example",
    };
  }

  getHealth(): HealthResponse {
    return {
      ok: true,
    };
  }
}
