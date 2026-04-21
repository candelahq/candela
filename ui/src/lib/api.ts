import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { TraceService } from "@/gen/candela/v1/trace_service_pb";
import { DashboardService } from "@/gen/candela/v1/dashboard_service_pb";
import { ProjectService } from "@/gen/candela/v1/project_service_pb";
import { UserService } from "@/gen/candela/v1/user_service_pb";

// Singleton clients — reuse across hooks.
export const traceClient = createClient(TraceService, transport);
export const dashboardClient = createClient(DashboardService, transport);
export const projectClient = createClient(ProjectService, transport);
export const userClient = createClient(UserService, transport);
