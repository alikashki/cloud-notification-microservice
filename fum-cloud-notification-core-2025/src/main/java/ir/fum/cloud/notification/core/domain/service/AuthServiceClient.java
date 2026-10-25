package ir.fum.cloud.notification.core.domain.service;

import auth.Auth;
import auth.AuthServiceGrpc;
import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import io.grpc.StatusRuntimeException;
import ir.fum.cloud.notification.core.configuration.ProjectConfiguration;
import ir.fum.cloud.notification.core.exception.NotificationException;
import ir.fum.cloud.notification.core.exception.NotificationExceptionStatus;
import org.springframework.stereotype.Service;

@Service
public class AuthServiceClient {


    private final AuthServiceGrpc.AuthServiceBlockingStub authServiceStub;
    private final ProjectConfiguration projectConfiguration;

    public AuthServiceClient(ProjectConfiguration projectConfiguration) {
        this.projectConfiguration = projectConfiguration;

        ManagedChannel channel = ManagedChannelBuilder
                .forAddress(
                        projectConfiguration.getAuthServiceGrpcHost(),
                        projectConfiguration.getAuthServiceGrpcPort()
                )
                .usePlaintext()
                .build();

        authServiceStub = AuthServiceGrpc.newBlockingStub(channel);

    }

    public Auth.ValidateResponse validateToken(String token) throws NotificationException {

        Auth.ValidateRequest request = Auth.ValidateRequest.newBuilder()
                .setToken(token)
                .build();

        try {
            // Call the gRPC Validate method
            return authServiceStub.validate(request);

        } catch (StatusRuntimeException e) {
            switch (e.getStatus().getCode()) {
                case UNAVAILABLE:
                    throw NotificationException.exception(
                            NotificationExceptionStatus.INTERNAL_ERROR,
                            "Auth grpc service is not available on "
                    );
                default:
                    throw NotificationException.exception(NotificationExceptionStatus.UNAUTHORIZED, e.getMessage());
            }
        }
    }
}
