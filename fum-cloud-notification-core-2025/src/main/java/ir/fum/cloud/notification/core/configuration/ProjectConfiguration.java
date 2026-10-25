package ir.fum.cloud.notification.core.configuration;

import lombok.Getter;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Configuration;

@Getter
@Configuration
public class ProjectConfiguration {

    @Value("${project.url.config}")
    private String projectUrlConfig;


    @Value("${auth.service.base.url:http://localhost:8082/}")
    private String authServiceUrl;

    @Value("${auth.grpc.address}")
    private String authServiceGrpcHost;

    @Value("${auth.grpc.port}")
    private int authServiceGrpcPort;

}
