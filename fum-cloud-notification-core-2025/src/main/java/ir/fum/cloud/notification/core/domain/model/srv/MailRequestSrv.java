package ir.fum.cloud.notification.core.domain.model.srv;


import lombok.Getter;
import lombok.NoArgsConstructor;
import lombok.Setter;
import lombok.experimental.SuperBuilder;

import java.util.List;

@Getter
@Setter
@NoArgsConstructor
@SuperBuilder
public class MailRequestSrv extends BaseSrv {
    private List<MailItemSrv> items;
    private String requestId;
    private String configName;
    private String content;
    private String plainText;
    private String subject;
}
