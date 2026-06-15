package scaffold

// templateYAML is the starter object template written to templates/configmap.yaml.
const templateYAML = `# Go template, rendered with Sprig functions before each step that uses it.
# Available variables (reference them as {{ "{{ .name }}" }}):
#   .name          the object's key from the case "objects" map
#   .<key>         any key from an ensure step's "values"
#   .Values.<key>  a value from the shared values store
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .name }}
  # labels:
  #   app: {{ .name }}
data:
  env: {{ .env | default "development" | quote }}
`

// caseYAML is the starter case written to cases/example.yaml. The uncommented
// fields form a minimal, runnable case; every optional field is shown as a
// comment so it can be uncommented as needed.
const caseYAML = `# kube2e test case. The uncommented fields below run as-is; uncomment the
# others to use them. Parsing is strict, so an unknown uncommented field errors.

# version must be "v1".
version: v1
name: example
description: A starter case — edit the steps below and uncomment options as needed.

# tags filter which cases run: "kube2e run <dir> --tags smoke".
# A case is skipped unless one of its tags is requested.
# tags:
#   - smoke

# namespace is created before the case runs (if absent) and reused by every
# object that does not set its own. kube2e never deletes it.
# namespace: kube2e-example

# objects maps a resource key to a template file in templates/ (without .yaml).
# The key is injected into the template as {{ .name }} and used as the object name.
objects:
  app: configmap

steps:
  - name: create
    # description: optional, shown in output
    # optional: true            # report a failure as skipped and keep going
    ensure:
      object: app
      values:
        env: staging
      # retry:
      #   attempts: 3           # total tries (1 = no retry)
      #   backoff: 2s
      # delay: 1s               # wait before running the action
      # timeout: 30s
    assert:
      target:
        object: app
      conditions:               # JQ expressions; all must be true
        - .data.env == "staging"
      # retry:
      #   attempts: 5
      #   backoff: 2s

  # --------------------------------------------------------------------------
  # A step runs its actions in this fixed order, skipping any that are absent:
  #   ensure -> patch -> wait -> assert -> logs -> exec -> delete
  # --------------------------------------------------------------------------

  # - name: patch
  #   patch:
  #     target:
  #       object: app
  #     patches:
  #       - op: add               # add | replace | remove | move | copy | test
  #         path: /metadata/labels/tier
  #         value: backend
  #     # retry:
  #     #   attempts: 3
  #     #   backoff: 2s

  # - name: wait
  #   wait:
  #     target:
  #       object: app
  #     conditions:
  #       - .status.phase == "Running"
  #     # interval: 2s
  #     # timeout: 2m

  # - name: logs
  #   logs:
  #     target:
  #       object: app             # Pod, Deployment, ReplicaSet, or StatefulSet
  #       # — or select existing pods by label instead of a templated object:
  #       # kind: Pod
  #       # labelSelector: app=demo
  #     contains: "started"
  #     # container: app
  #     # match: any              # any | all | none
  #     # interval: 2s
  #     # timeout: 2m

  # - name: exec
  #   exec:
  #     target:
  #       object: app             # — or kind: Pod + labelSelector: app=demo
  #     command: ["sh", "-c", "echo hello"]
  #     # container: app
  #     # timeout: 30s
  #     # retry:
  #     #   attempts: 3
  #     #   backoff: 2s

  # - name: delete
  #   delete:
  #     target:
  #       object: app
  #     # wait: true              # block until the object is gone
  #     # interval: 2s
  #     # timeout: 2m

# hooks run around every step in this case.
# hooks:
#   beforeEach:
#     - name: reset
#       delete:
#         target:
#           object: app
#         wait: true
#   afterEach:
#     - name: verify
#       assert:
#         target:
#           object: app
#         conditions:
#           - .metadata.name == "app"
`
