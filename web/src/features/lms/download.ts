import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { agoraApi } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { openDownload } from '@/shared/lib/download'

/** useDownload, etkinliğin dosyasını imzalı URL ile indirir (tarayıcı belleğine alınmaz). */
export function useDownload() {
  const dispatch = useAppDispatch()
  const message = useErrorMessage()
  return async (activityId: string, fileId: string) => {
    try {
      const res = await dispatch(
        agoraApi.endpoints.getActivityFileUrl.initiate(
          { id: activityId, fileId },
          { subscribe: false, forceRefetch: true },
        ),
      ).unwrap()
      openDownload(res.url)
    } catch (e) {
      toast.error(message(e as never))
    }
  }
}
