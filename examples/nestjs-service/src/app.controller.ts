import { Controller, Get } from "@nestjs/common";
import {
  AppService,
  type HealthResponse,
  type RootResponse,
} from "./app.service";

const appService = new AppService();

@Controller()
export class AppController {
  @Get()
  getRoot(): RootResponse {
    return appService.getRoot();
  }

  @Get("health")
  getHealth(): HealthResponse {
    return appService.getHealth();
  }
}
