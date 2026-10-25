package ir.fum.cloud.notification.core.domain.model.srv;


import ir.fum.cloud.notification.core.data.hibernate.entity.model.RequestState;
import lombok.Getter;
import lombok.NoArgsConstructor;
import lombok.Setter;
import lombok.experimental.SuperBuilder;

@Getter
@Setter
@NoArgsConstructor
@SuperBuilder
public class MailItemSrv extends BaseSrv {
    private RequestState requestState;
    private String receiver;
    private long sendDateTimestamp;
}
