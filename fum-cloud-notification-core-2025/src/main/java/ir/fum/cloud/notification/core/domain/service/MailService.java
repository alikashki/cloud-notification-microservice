package ir.fum.cloud.notification.core.domain.service;


import ir.fum.cloud.notification.core.data.hibernate.entity.MailRequest;
import ir.fum.cloud.notification.core.data.hibernate.repository.MailRequestRepository;
import ir.fum.cloud.notification.core.domain.annotation.Pagination;
import ir.fum.cloud.notification.core.domain.model.helper.GenericResponse;
import ir.fum.cloud.notification.core.domain.model.srv.MailItemSrv;
import ir.fum.cloud.notification.core.domain.model.srv.MailRequestSrv;
import ir.fum.cloud.notification.core.domain.model.vo.UserVO;
import ir.fum.cloud.notification.core.util.GeneralUtils;
import ir.fum.cloud.notification.core.util.request.ResponseUtil;
import lombok.RequiredArgsConstructor;
import lombok.extern.log4j.Log4j2;
import org.hibernate.Session;
import org.hibernate.SessionFactory;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.validation.annotation.Validated;

import java.util.ArrayList;
import java.util.List;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
@Log4j2
@Validated
public class MailService {
    private final MailRequestRepository mailRequestRepository;
    private final SessionFactory sessionFactory;


    @Transactional(value = "projectTransactionManager", rollbackFor = Exception.class)
    public GenericResponse<List<MailRequestSrv>> getMailRequests(UserVO user,
                                                                 @Pagination int size,
                                                                 @Pagination int offset) {

        Session session = sessionFactory.getCurrentSession();

        long totalCount = mailRequestRepository.getCountRequestsByUserId(session, user.getUser_id());

        List<MailRequestSrv> requests = new ArrayList<>();

        if (totalCount > 0 && offset < totalCount) {
            List<MailRequest> records = mailRequestRepository.getRequestsByUserId(session, user.getUser_id(), size, offset);

            requests = records.stream()
                    .map(this::getSrv)
                    .collect(Collectors.toList());
        }

        return ResponseUtil.getResponse(requests, totalCount);
    }


    private MailRequestSrv getSrv(MailRequest mailRequest) {
        return MailRequestSrv.builder()
                .content(mailRequest.getBody())
                .subject(mailRequest.getSubject())
                .plainText(mailRequest.getText())
                .requestId(mailRequest.getRequest().getNotificationId())
                .configName(mailRequest.getMailConfig().getName())
                .items(mailRequest.getMailItemSet().stream()
                        .map(mailItem -> MailItemSrv.builder()
                                .requestState(mailItem.getState())
                                .receiver(mailItem.getReceiver())
                                .sendDateTimestamp(GeneralUtils.getTimestamp(mailItem.getSendDate()))
                                .build())
                        .collect(Collectors.toList()))
                .build();
    }

}
