package ir.fum.cloud.notification.core.data.hibernate.repository;

import ir.fum.cloud.notification.core.data.hibernate.entity.MailRequest;
import ir.fum.cloud.notification.core.data.hibernate.entity.model.MessageType;
import lombok.RequiredArgsConstructor;
import org.hibernate.Session;
import org.hibernate.SessionFactory;
import org.springframework.stereotype.Repository;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Repository
@RequiredArgsConstructor
public class MailRequestRepository extends BaseRepository<MailRequest> {
    private final SessionFactory sessionFactory;

    @Transactional(value = "projectTransactionManager", rollbackFor = Exception.class)
    public MailRequest findByRequestId(String requestId) {

        return sessionFactory.getCurrentSession()
                .createQuery("select mailRequest " +
                                "from MailRequest mailRequest " +
                                "left join fetch mailRequest.request request " +
                                "where " +
                                "request.notificationId = :requestId " +
                                "and " +
                                "request.messageType = :messageType",
                        MailRequest.class)
                .setParameter("requestId", requestId)
                .setParameter("messageType", MessageType.SEND_EMAIL_MESSAGE_TYPE.getType())
                .uniqueResult();
    }

    public long getCountRequestsByUserId(Session session, long userId) {
        return session.createQuery("select count(request.id) " +
                                "from MailRequest request " +
                                "left join request.mailConfig config " +
                                "where " +
                                "config.userId = :userId ",
                        Long.class)
                .setParameter("userId", userId)
                .uniqueResult();
    }

    public List<MailRequest> getRequestsByUserId(Session session, long userId, int size, int offset) {
        int maxResult = size == 0 ? 200 : size;

        return session.createQuery("select request " +
                                "from MailRequest request " +
                                "left join fetch request.mailConfig config " +
                                "left join fetch request.request " +
                                "left join fetch request.mailItemSet " +
                                "where " +
                                "config.userId = :userId ",
                        MailRequest.class)
                .setParameter("userId", userId)
                .setFirstResult(offset)
                .setMaxResults(maxResult)
                .list();

    }
}
